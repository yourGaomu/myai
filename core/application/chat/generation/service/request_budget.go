package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	compactionport "myai/core/application/chat/compaction/port"
	chatcontextport "myai/core/application/chat/context/port"
	generationcommand "myai/core/application/chat/generation/command"
	"myai/core/contextmgr"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
)

func requestBudget(command generationcommand.Run, tools []modelport.Tool) (contextmgr.RequestBudget, error) {
	toolTokens, err := estimateToolTokens(tools)
	if err != nil {
		return contextmgr.RequestBudget{}, err
	}
	return contextmgr.NewRequestBudget(contextmgr.ResolveWindowTokens(command.Session.ContextWindowK, command.ModelWindowTokens), toolTokens, command.Settings.MaxOutputTokens), nil
}

func (s AgentLoopService) prepareSnapshot(ctx context.Context, command generationcommand.Run, budget contextmgr.RequestBudget) (contextmgr.Snapshot, error) {
	var err error
	turn := contextmgr.TurnContext{EnvironmentContext: command.EnvironmentContext, PlanContext: planSnapshot(command.Session.CurrentPlan)}
	// Legacy transcripts keep their raw messages and checkpoint hash. Only the
	// latest turn's dynamic instructions are projected into this request.
	latest := contextmgr.LatestUserInputIndex(command.Session.Messages)
	start := latest
	for start > 0 && contextmgr.IsDynamicContext(command.Session.Messages[start-1]) {
		start--
	}
	for index := max(0, start); index < len(command.Session.Messages); index++ {
		message := command.Session.Messages[index]
		switch message.SyntheticReason {
		case domainmessage.SyntheticReasonRuntimeInstruction:
			turn.RuntimeInstruction, _ = message.RuntimeInstructionText()
		case domainmessage.SyntheticReasonRAGContext:
			turn.RAGContext = strings.TrimSpace(strings.TrimPrefix(message.Text(), domainmessage.RAGContextPrefix))
		}
	}
	options := contextmgr.RequestOptions{Budget: budget, TurnContext: turn}
	if s.Compactor != nil {
		if compactor, ok := s.Compactor.(compactionport.RequestCompactor); ok {
			_, err = compactor.CompactIfNeededForRequest(ctx, command.Session, command.Model, options)
		} else {
			err = s.compactIfNeeded(ctx, command, budget.ReservedTokens())
		}
		if err != nil {
			if s.OnCompactError != nil {
				s.OnCompactError(err)
			}
			return contextmgr.Snapshot{}, fmt.Errorf("compact request context: %w", err)
		}
	}
	var snapshot contextmgr.Snapshot
	if provider, ok := s.Contexts.(chatcontextport.RequestProvider); ok {
		snapshot = provider.SnapshotForRequest(command.Session, options)
	} else if provider, ok := s.Contexts.(chatcontextport.BudgetedProvider); ok {
		snapshot = provider.SnapshotWithBudget(command.Session, budget.ReservedTokens())
	} else {
		snapshot = s.Contexts.Snapshot(command.Session)
		if environment := domainmessage.EnvironmentContext(command.EnvironmentContext); environment.IsSynthetic() {
			latest := contextmgr.LatestUserInputIndex(snapshot.Messages)
			if latest < 0 {
				latest = len(snapshot.Messages)
			}
			messages := domainmessage.CloneAll(snapshot.Messages[:latest])
			messages = append(messages, environment)
			snapshot.Messages = append(messages, domainmessage.CloneAll(snapshot.Messages[latest:])...)
		}
	}
	if snapshot.Info.WindowK > 0 {
		budget.WindowTokens = min(budget.WindowTokens, snapshot.Info.WindowK*1000)
	}
	if snapshot.Info.SelectedTokens+budget.ReservedTokens() > budget.WindowTokens {
		return snapshot, fmt.Errorf("context window exceeded: messages=%d, reserved=%d, window=%d", snapshot.Info.SelectedTokens, budget.ReservedTokens(), budget.WindowTokens)
	}
	if err := quarantineInvalidToolExchanges(&snapshot); err != nil {
		return snapshot, err
	}
	if err := budget.ValidateSnapshot(snapshot); err != nil {
		return snapshot, fmt.Errorf("context window validation failed: %w", err)
	}
	return snapshot, nil
}

// quarantineInvalidToolExchanges removes a whole completed exchange from the
// request projection. Durable history and compaction source indexes stay intact.
func quarantineInvalidToolExchanges(snapshot *contextmgr.Snapshot) error {
	invalid := make(map[string]bool)
	for _, message := range snapshot.Messages {
		for _, part := range message.Parts {
			if part.ToolCall == nil {
				continue
			}
			call := part.ToolCall
			var object map[string]json.RawMessage
			if err := json.Unmarshal([]byte(call.Arguments), &object); err != nil || object == nil {
				for _, sibling := range message.Parts {
					if sibling.ToolCall != nil {
						invalid[sibling.ToolCall.ID] = true
					}
				}
			}
		}
	}
	if len(invalid) == 0 {
		return nil
	}
	remaining := make(map[string]bool, len(invalid))
	for id := range invalid {
		remaining[id] = true
	}
	projected := make([]domainmessage.Message, 0, len(snapshot.Messages))
	for _, message := range snapshot.Messages {
		skip := false
		for _, part := range message.Parts {
			if part.ToolCall != nil && invalid[part.ToolCall.ID] {
				skip = true
			}
			if part.ToolResult != nil && invalid[part.ToolResult.ToolCallID] {
				skip = true
				delete(remaining, part.ToolResult.ToolCallID)
			}
		}
		if !skip {
			projected = append(projected, domainmessage.Clone(message))
		}
	}
	if len(remaining) > 0 {
		return fmt.Errorf("invalid historical tool exchange has missing results; regenerate the affected turn")
	}
	projected = append(projected, domainmessage.SyntheticUserText(domainmessage.SyntheticReasonHookContext, "A historical tool exchange with invalid JSON arguments was omitted from this request. Its raw transcript is preserved. Inspect the workspace again before relying on that exchange."))
	snapshot.Messages = projected
	snapshot.Info.SelectedTokens = contextmgr.EstimateMessagesTokens(projected)
	return nil
}

func (s AgentLoopService) generateSample(ctx context.Context, command generationcommand.Run, tools []modelport.Tool, images *turnImages, retried *bool) (modelport.ChatResult, contextmgr.Snapshot, error) {
	budget, err := requestBudget(command, tools)
	if err != nil {
		return modelport.ChatResult{}, contextmgr.Snapshot{}, err
	}
	build := func(budget contextmgr.RequestBudget) (modelport.GenerateRequest, contextmgr.Snapshot, int, error) {
		snapshot, err := s.prepareSnapshot(ctx, command, budget)
		if err != nil {
			return modelport.GenerateRequest{}, snapshot, 0, err
		}
		if snapshot.Info.WindowK > 0 {
			budget.WindowTokens = min(budget.WindowTokens, snapshot.Info.WindowK*1000)
		}
		request := modelport.GenerateRequest{Messages: snapshot.Messages, Tools: tools, Stream: command.Stream, Settings: command.Settings}
		imageTokens, err := s.addRequestImages(ctx, command.Session.ID, command.Model, images, snapshot, command.Session.Messages, budget, &request)
		return request, snapshot, imageTokens, err
	}
	request, snapshot, imageTokens, err := build(budget)
	if err != nil {
		return modelport.ChatResult{}, snapshot, err
	}
	result, err := command.Model.Generate(ctx, request)
	if !modelport.IsContextWindowExceeded(err) || retried == nil || *retried {
		return result, snapshot, err
	}
	*retried = true
	if _, ok := s.Compactor.(compactionport.RequestCompactor); !ok {
		return result, snapshot, err
	}
	// Lower the accounting window to force an additional persisted checkpoint,
	// then retry only the model sample. Rebuild image bindings from the new
	// snapshot and reuse any bytes already verified during this turn.
	requestTokens := contextmgr.EstimateMessagesTokens(request.Messages) + imageTokens
	budget.WindowTokens = min(budget.WindowTokens*3/4, budget.ReservedTokens()+max(256, requestTokens/2))
	retry, rebuilt, retryImageTokens, compactErr := build(budget)
	if compactErr != nil {
		return result, snapshot, fmt.Errorf("model context overflow; recovery compaction failed: %w", compactErr)
	}
	if contextmgr.EstimateMessagesTokens(retry.Messages)+retryImageTokens >= requestTokens {
		return result, snapshot, err
	}
	result, err = command.Model.Generate(ctx, retry)
	return result, rebuilt, err
}

func estimateToolTokens(tools []modelport.Tool) (int, error) {
	if len(tools) == 0 {
		return 0, nil
	}
	encoded, err := json.Marshal(tools)
	if err != nil {
		return 0, fmt.Errorf("estimate tool context: %w", err)
	}
	return contextmgr.EstimateTextTokens(string(encoded)), nil
}
