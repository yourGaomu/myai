package service

import (
	"context"
	"errors"
	"strings"

	generationapi "myai/core/application/chat/generation/api"
	generationcommand "myai/core/application/chat/generation/command"
	generationport "myai/core/application/chat/generation/port"
	generationresult "myai/core/application/chat/generation/result"
	"myai/core/contextmgr"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
	"myai/core/session"
)

const DefaultMaxToolRounds = 32
const maxStopHookContinuations = 3

type AgentLoopService struct {
	// AgentLoopService 实现“模型 -> 工具 -> 模型”的循环，直到模型不再请求工具。
	Contexts       generationport.ContextProvider
	Tools          generationport.ToolCatalog
	ToolExecutor   generationport.ToolExecutor
	ToolRecords    generationport.ToolExecutionRecordSink
	Compactor      generationport.AutoCompactor
	TurnHooks      generationport.TurnLifecycleHooks
	PendingInput   generationport.PendingTurnInput
	MaxToolRounds  int
	OnCompactError func(error)
}

var _ generationapi.AgentRunner = AgentLoopService{}

func (s AgentLoopService) Run(ctx context.Context, command generationcommand.Run) (modelport.ChatResult, error) {
	if command.Model == nil {
		return modelport.ChatResult{}, errors.New("model is nil")
	}
	if command.Session == nil {
		return modelport.ChatResult{}, errors.New("session is nil")
	}
	if s.Contexts == nil {
		return modelport.ChatResult{}, errors.New("context provider is nil")
	}

	totalUsage := modelport.TokenUsage{}
	maxToolRounds := s.maxToolRounds(command.Session)
	reasoningParts := make([]string, 0, maxToolRounds)
	stopContinuations := 0
	canDrainPending := false
	for round := 0; round < maxToolRounds; round++ {
		if canDrainPending {
			s.drainPendingInput(command.Session)
		}
		if err := s.compactIfNeeded(ctx, command); err != nil {
			return modelport.ChatResult{}, err
		}
		snapshot := s.Contexts.Snapshot(command.Session)
		if err := validateContextWindow(snapshot); err != nil {
			return modelport.ChatResult{}, err
		}
		// 每轮都重新构建快照，因为上一轮可能追加了 tool call 和 tool result。
		result, err := command.Model.Generate(ctx, modelport.GenerateRequest{
			Messages: withTurnContexts(snapshot.Messages, command.EnvironmentContext, command.MemoryContext),
			Tools:    s.toolsForSession(command.Session, command.ForceChatMode),
			Stream:   command.Stream,
			Settings: command.Settings,
		})
		if err != nil {
			return modelport.ChatResult{}, err
		}
		canDrainPending = true

		totalUsage = totalUsage.Add(result.Usage)
		reasoningParts = appendReasoningPart(reasoningParts, result.Reasoning)
		if len(result.ToolCalls) == 0 {
			continued, hookErr := s.continueAfterStopHook(ctx, command, result.Content, &stopContinuations)
			if hookErr != nil {
				return modelport.ChatResult{}, hookErr
			}
			if continued || s.hasPendingInput(command.Session) {
				continue
			}
			// 没有工具调用表示模型已经给出最终回答，汇总所有轮次的 usage 和 reasoning 后结束。
			return finalizeResult(result, totalUsage, reasoningParts), nil
		}

		toolResult, err := s.executeTools(ctx, generationcommand.ToolExecution{
			Session: command.Session, Calls: result.ToolCalls, Stream: command.Stream, RequestID: command.RequestID,
			ForceChatMode: command.ForceChatMode,
		})
		s.recordToolExecution(ctx, toolResult)
		// 工具调用与结果都进入会话，下一轮模型才能基于真实执行结果继续推理。
		// Calls 来自执行层，Hook 改写参数后的值与真实调用、持久化记录完全一致。
		calls := toolResult.Calls
		// 兼容外部 ToolExecutor：旧实现未提供 Calls 时仍保留模型调用消息；正式执行器返回非 nil 的实际调用列表。
		if calls == nil {
			calls = result.ToolCalls
		}
		if len(calls) > 0 {
			command.Session.AppendMessage(domainmessage.ToolCallMessage(calls))
		}
		command.Session.AppendMessages(toolResult.Messages...)
		if err != nil {
			// 工具批次可部分完成；已经执行的记录必须先提交，再终止本轮避免模型重试副作用工具。
			return modelport.ChatResult{}, err
		}
		if hook := generationcommand.AfterToolRoundFrom(ctx); hook != nil {
			if hookErr := hook(ctx, command.Session); hookErr != nil {
				return modelport.ChatResult{}, hookErr
			}
		}
	}

	s.drainPendingInput(command.Session)
	if err := s.compactIfNeeded(ctx, command); err != nil {
		return modelport.ChatResult{}, err
	}
	snapshot := s.Contexts.Snapshot(command.Session)
	if err := validateContextWindow(snapshot); err != nil {
		return modelport.ChatResult{}, err
	}

	// 达到工具轮数上限后进行一次无工具生成，避免模型无限调用工具。
	result, err := command.Model.Generate(ctx, modelport.GenerateRequest{
		Messages: withTurnContexts(snapshot.Messages, command.EnvironmentContext, command.MemoryContext),
		Stream:   command.Stream,
		Settings: command.Settings,
	})
	if err != nil {
		return modelport.ChatResult{}, err
	}
	totalUsage = totalUsage.Add(result.Usage)
	reasoningParts = appendReasoningPart(reasoningParts, result.Reasoning)
	return finalizeResult(result, totalUsage, reasoningParts), nil
}

func withTurnContexts(messages []domainmessage.Message, environmentPrompt, memoryPrompt string) []domainmessage.Message {
	extras := make([]domainmessage.Message, 0, 2)
	if environment := domainmessage.EnvironmentContext(environmentPrompt); environment.IsSynthetic() {
		extras = append(extras, environment)
	}
	if memory := domainmessage.MemoryContext(memoryPrompt); memory.IsSynthetic() {
		extras = append(extras, memory)
	}
	if len(extras) == 0 {
		return messages
	}
	insertAt := len(messages)
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == domainmessage.RoleUser {
			insertAt = index
			break
		}
	}
	result := make([]domainmessage.Message, 0, len(messages)+len(extras))
	result = append(result, messages[:insertAt]...)
	result = append(result, extras...)
	result = append(result, messages[insertAt:]...)
	return result
}

func validateContextWindow(snapshot contextmgr.Snapshot) error {
	if snapshot.Info.WindowK <= 0 {
		return nil
	}
	if snapshot.Info.SelectedTokens <= snapshot.Info.WindowK*1000 {
		return nil
	}
	return errors.New("current conversation turn exceeds the configured context window; shorten the request or tool output, or increase the session context window")
}

func (s AgentLoopService) maxToolRounds(current *session.Session) int {
	if current != nil && current.MaxToolRounds > 0 {
		return current.MaxToolRounds
	}
	if s.MaxToolRounds > 0 {
		return s.MaxToolRounds
	}
	return DefaultMaxToolRounds
}

func (s AgentLoopService) toolsForSession(current *session.Session, forceChatMode bool) []modelport.Tool {
	if s.Tools == nil {
		return nil
	}
	return s.Tools.ToolsForSession(current, forceChatMode)
}

func (s AgentLoopService) drainPendingInput(current *session.Session) {
	if s.PendingInput == nil || current == nil || strings.TrimSpace(current.ID) == "" {
		return
	}
	for _, content := range s.PendingInput.Drain(current.ID) {
		current.AppendMessage(domainmessage.Text(domainmessage.RoleUser, content))
	}
}

func (s AgentLoopService) hasPendingInput(current *session.Session) bool {
	if s.PendingInput == nil || current == nil {
		return false
	}
	return s.PendingInput.HasPending(current.ID)
}

func (s AgentLoopService) compactIfNeeded(ctx context.Context, command generationcommand.Run) error {
	if s.Compactor == nil || command.Session == nil || command.Model == nil {
		return nil
	}
	_, err := s.Compactor.CompactIfNeeded(ctx, command.Session, command.Model)
	if err == nil {
		return nil
	}
	if s.OnCompactError != nil {
		s.OnCompactError(err)
	}
	return nil
}

func (s AgentLoopService) continueAfterStopHook(ctx context.Context, command generationcommand.Run, lastAssistant string, stopContinuations *int) (bool, error) {
	if s.TurnHooks == nil || command.Session == nil || stopContinuations == nil || *stopContinuations >= maxStopHookContinuations {
		return false, nil
	}
	outcome, err := s.TurnHooks.Handle(ctx, generationcommand.TurnHook{
		Kind:          generationcommand.TurnHookStop,
		SessionID:     command.Session.ID,
		LastAssistant: lastAssistant,
	})
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(outcome.Continuation) == "" {
		return false, nil
	}
	message := domainmessage.HookContext(outcome.Continuation)
	if !message.IsSynthetic() {
		return false, nil
	}
	command.Session.AppendMessage(message)
	*stopContinuations++
	return true, nil
}

func (s AgentLoopService) executeTools(ctx context.Context, command generationcommand.ToolExecution) (generationresult.ToolExecution, error) {
	if s.ToolExecutor == nil {
		return generationresult.ToolExecution{}, errors.New("tool executor is nil")
	}
	return s.ToolExecutor.Execute(ctx, command)
}

func (s AgentLoopService) recordToolExecution(ctx context.Context, result generationresult.ToolExecution) {
	if s.ToolRecords == nil || len(result.Entries) == 0 && len(result.Assets) == 0 {
		return
	}
	s.ToolRecords.RecordToolExecution(ctx, generationcommand.ToolExecutionRecord{Entries: result.Entries, Assets: result.Assets})
}

func (s AgentLoopService) RecordToolExecution(ctx context.Context, result generationresult.ToolExecution) {
	s.recordToolExecution(ctx, result)
}

func finalizeResult(result modelport.ChatResult, usage modelport.TokenUsage, reasoningParts []string) modelport.ChatResult {
	result.Usage = usage
	result.Reasoning = strings.Join(reasoningParts, "\n")
	return result
}

func appendReasoningPart(parts []string, reasoning string) []string {
	reasoning = strings.TrimSpace(reasoning)
	if reasoning == "" {
		return parts
	}
	return append(parts, reasoning)
}
