package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	generationapi "myai/core/application/chat/generation/api"
	generationcommand "myai/core/application/chat/generation/command"
	generationport "myai/core/application/chat/generation/port"
	generationresult "myai/core/application/chat/generation/result"
	"myai/core/contextmgr"
	domaingeneration "myai/core/domain/generation"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
	"myai/core/session"
	toolruntime "myai/core/tool/runtimecontext"
)

const DefaultMaxToolRounds = 32
const DefaultMaxKnowledgeSearchCalls = 2
const DefaultMaxMemorySearchCalls = 1
const maxStopHookContinuations = 3

// MailboxDeliveryPhase describes when queued inter-agent messages may enter
// the model context. Keeping this as an explicit phase prevents a boolean
// from hiding the difference between queue-only delivery and turn steering.
type MailboxDeliveryPhase string

const (
	// MailboxDeferred means the current model turn keeps its context unchanged.
	MailboxDeferred MailboxDeliveryPhase = "deferred"
	// MailboxAcceptCurrentTurn means a steer message may be consumed before the
	// next model sample in the active turn.
	MailboxAcceptCurrentTurn MailboxDeliveryPhase = "accept_current_turn"
	// MailboxNextTurn means the current answer reached a boundary and queued
	// messages may be consumed by the next model sample.
	MailboxNextTurn MailboxDeliveryPhase = "next_turn"
)

func (phase MailboxDeliveryPhase) shouldDrain() bool {
	return phase == MailboxAcceptCurrentTurn || phase == MailboxNextTurn
}

type AgentLoopService struct {
	// AgentLoopService 实现“模型 -> 工具 -> 模型”的循环，直到模型不再请求工具。
	Contexts            generationport.ContextProvider
	Tools               generationport.ToolCatalog
	ToolExecutor        generationport.ToolExecutor
	ToolRecords         generationport.ToolExecutionRecordSink
	Compactor           generationport.AutoCompactor
	TurnHooks           generationport.TurnLifecycleHooks
	PendingInput        generationport.PendingTurnInput
	OnPendingInputError func(error)
	// OnPendingInputAppended persists structured mailbox messages consumed while
	// a turn is already running. The callback is optional for isolated tests.
	OnPendingInputAppended  func(current *session.Session, messages []domainmessage.Message)
	MaxToolRounds           int
	MaxKnowledgeSearchCalls int
	MaxMemorySearchCalls    int
	OnCompactError          func(error)
}

var _ generationapi.AgentRunner = AgentLoopService{}

func (s AgentLoopService) Run(ctx context.Context, command generationcommand.Run) (response modelport.ChatResult, runErr error) {
	// 1. 先确认本轮生成具备模型、会话和上下文能力，再进入 mailbox/工具循环。
	if command.Model == nil {
		return modelport.ChatResult{}, errors.New("model is nil")
	}
	if command.Session == nil {
		return modelport.ChatResult{}, errors.New("session is nil")
	}
	if s.Contexts == nil {
		return modelport.ChatResult{}, errors.New("context provider is nil")
	}

	var consumedPending []domaingeneration.PendingTurnInputItem
	defer func() {
		// 2. 生成失败则释放已领取消息，生成成功才确认消息已经被本轮消费。
		if len(consumedPending) == 0 || command.Session == nil {
			return
		}
		if runErr != nil {
			s.releasePendingInput(command.Session.ID, consumedPending)
			return
		}
		s.acknowledgePendingInput(command.Session.ID, consumedPending)
	}()

	totalUsage := modelport.TokenUsage{}
	maxToolRounds := s.maxToolRounds(command.Session)
	runCtx := toolruntime.WithKnowledgeSearchBudget(ctx, toolruntime.NewKnowledgeSearchBudget(s.maxKnowledgeSearchCalls()))
	runCtx = toolruntime.WithMemorySearchBudget(runCtx, toolruntime.NewMemorySearchBudget(s.maxMemorySearchCalls()))
	reasoningParts := make([]string, 0, maxToolRounds)
	stopContinuations := 0
	// Queue-only inter-agent messages are delivered at an answer boundary. This
	// mirrors Codex mailbox semantics: tool execution keeps its current context
	// stable, while the next model turn sees the completed mailbox batch.
	mailboxPhase := MailboxDeferred
	for round := 0; round < maxToolRounds; round++ {
		if mailboxPhase.shouldDrain() {
			// 3. 只有显式 phase 允许时才领取 mailbox，避免工具执行中途改变上下文。
			pending, drainErr := s.drainPendingInput(command.Session)
			consumedPending = append(consumedPending, pending...)
			// 3.1 一批消息进入当前上下文后回到 deferred，下一次迁移必须由新的事件触发。
			mailboxPhase = MailboxDeferred
			if drainErr != nil {
				return modelport.ChatResult{}, drainErr
			}
		}
		if err := s.compactIfNeeded(runCtx, command); err != nil {
			return modelport.ChatResult{}, err
		}
		snapshot := s.Contexts.Snapshot(command.Session)
		if err := validateContextWindow(snapshot); err != nil {
			return modelport.ChatResult{}, err
		}
		// 每轮都重新构建快照，因为上一轮可能追加了 tool call 和 tool result。
		result, err := command.Model.Generate(runCtx, modelport.GenerateRequest{
			Messages: withTurnContexts(snapshot.Messages, command.EnvironmentContext),
			Tools:    s.toolsForSession(command.Session, command.ForceChatMode),
			Stream:   command.Stream,
			Settings: command.Settings,
		})
		if err != nil {
			return modelport.ChatResult{}, err
		}
		// 4. 模型无工具调用时形成回答边界；有工具调用时继续当前 turn。
		totalUsage = totalUsage.Add(result.Usage)
		reasoningParts = appendReasoningPart(reasoningParts, result.Reasoning)
		if len(result.ToolCalls) == 0 {
			continued, hookErr := s.continueAfterStopHook(runCtx, command, result.Content, &stopContinuations)
			if hookErr != nil {
				return modelport.ChatResult{}, hookErr
			}
			if continued {
				continue
			}
			if s.hasPendingInput(command.Session) {
				// 5. 回答边界后迁移到 next_turn，下一轮再把 queue 消息加入上下文。
				// The current answer is complete; consume queued mailbox messages
				// only on the following model round.
				mailboxPhase = MailboxNextTurn
				continue
			}
			// 没有工具调用表示模型已经给出最终回答，汇总所有轮次的 usage 和 reasoning 后结束。
			return finalizeResult(result, totalUsage, reasoningParts), nil
		}

		toolResult, err := s.executeTools(runCtx, generationcommand.ToolExecution{
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
		if hook := generationcommand.AfterToolRoundFrom(runCtx); hook != nil {
			if hookErr := hook(runCtx, command.Session); hookErr != nil {
				return modelport.ChatResult{}, hookErr
			}
		}
		// 6. steer_current_turn 可以打断当前工具链；普通 queue 消息继续等待回答边界。
		if s.hasImmediatePendingInput(command.Session) {
			// 6. steer 消息迁移到 accept_current_turn，允许下一次采样读取它。
			mailboxPhase = MailboxAcceptCurrentTurn
		}
	}

	pending, drainErr := s.drainPendingInput(command.Session)
	consumedPending = append(consumedPending, pending...)
	if drainErr != nil {
		return modelport.ChatResult{}, drainErr
	}
	if err := s.compactIfNeeded(runCtx, command); err != nil {
		return modelport.ChatResult{}, err
	}
	snapshot := s.Contexts.Snapshot(command.Session)
	if err := validateContextWindow(snapshot); err != nil {
		return modelport.ChatResult{}, err
	}

	// 达到工具轮数上限后进行一次无工具生成，避免模型无限调用工具。
	result, err := command.Model.Generate(runCtx, modelport.GenerateRequest{
		Messages: withTurnContexts(snapshot.Messages, command.EnvironmentContext),
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

func withTurnContexts(messages []domainmessage.Message, environmentPrompt string) []domainmessage.Message {
	extras := make([]domainmessage.Message, 0, 1)
	if environment := domainmessage.EnvironmentContext(environmentPrompt); environment.IsSynthetic() {
		extras = append(extras, environment)
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

func (s AgentLoopService) maxKnowledgeSearchCalls() int {
	if s.MaxKnowledgeSearchCalls > 0 {
		return s.MaxKnowledgeSearchCalls
	}
	return DefaultMaxKnowledgeSearchCalls
}

func (s AgentLoopService) maxMemorySearchCalls() int {
	if s.MaxMemorySearchCalls > 0 {
		return s.MaxMemorySearchCalls
	}
	return DefaultMaxMemorySearchCalls
}

func (s AgentLoopService) toolsForSession(current *session.Session, forceChatMode bool) []modelport.Tool {
	if s.Tools == nil {
		return nil
	}
	return s.Tools.ToolsForSession(current, forceChatMode)
}

func (s AgentLoopService) drainPendingInput(current *session.Session) ([]domaingeneration.PendingTurnInputItem, error) {
	if s.PendingInput == nil || current == nil || strings.TrimSpace(current.ID) == "" {
		return nil, nil
	}
	items := make([]domaingeneration.PendingTurnInputItem, 0)
	appended := make([]domainmessage.Message, 0)
	var conflictErr error
	if identified, ok := s.PendingInput.(generationport.IdentifiedPendingTurnInput); ok {
		items = identified.DrainIdentified(current.ID)
		for _, item := range items {
			if item.ID != "" {
				// 1. 先用 SourceID 检查消息是否已经持久化，避免重试重复追加。
				alreadyApplied := false
				for _, existing := range current.Messages {
					if existing.SourceID != item.ID {
						continue
					}
					alreadyApplied = true
					if existing.Text() != item.Content && s.OnPendingInputError != nil {
						s.OnPendingInputError(fmt.Errorf("pending message id %q conflicts with session history", item.ID))
					}
					if existing.Text() != item.Content && conflictErr == nil {
						conflictErr = fmt.Errorf("pending message id %q conflicts with session history", item.ID)
					}
					break
				}
				if alreadyApplied {
					// 1.1 相同来源已经进入会话，重试只需要确认消息，不再追加副本。
					continue
				}
			}
			// 2. pending 消息可能来自子代理，追加时必须保留事件来源，不能降级为匿名文本。
			message := domainmessage.SyntheticUserText(domainmessage.SyntheticReasonSubagentResult, item.Content)
			// 2.1 追加时保留来源字段，后续才能继续进行幂等确认和冲突排查。
			message.SourceID = item.ID
			message.SourceKind = item.SourceKind
			message.SourceTaskID = item.SourceTaskID
			current.AppendMessage(message)
			appended = append(appended, domainmessage.Clone(message))
		}
		if len(appended) > 0 && s.OnPendingInputAppended != nil {
			// 3. 先把新增消息交给持久化适配器，再等待本轮最终回答完成确认。
			s.OnPendingInputAppended(current, domainmessage.CloneAll(appended))
		}
		return items, conflictErr
	}
	for _, content := range s.PendingInput.Drain(current.ID) {
		message := domainmessage.Text(domainmessage.RoleUser, content)
		current.AppendMessage(message)
		appended = append(appended, domainmessage.Clone(message))
		items = append(items, domaingeneration.PendingTurnInputItem{Content: content})
	}
	if len(appended) > 0 && s.OnPendingInputAppended != nil {
		s.OnPendingInputAppended(current, domainmessage.CloneAll(appended))
	}
	return items, nil
}

func (s AgentLoopService) acknowledgePendingInput(sessionID string, items []domaingeneration.PendingTurnInputItem) {
	identified, ok := s.PendingInput.(generationport.IdentifiedPendingTurnInput)
	if !ok {
		return
	}
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		if err := identified.Acknowledge(item.ID); err != nil {
			s.releasePendingInput(sessionID, []domaingeneration.PendingTurnInputItem{item})
			if s.OnPendingInputError != nil {
				s.OnPendingInputError(err)
			}
		}
	}
}

func (s AgentLoopService) releasePendingInput(sessionID string, items []domaingeneration.PendingTurnInputItem) {
	if len(items) == 0 || s.PendingInput == nil {
		return
	}
	if releaser, ok := s.PendingInput.(generationport.PendingInputReleaser); ok {
		if err := releaser.RequeueIdentified(sessionID, items); err != nil && s.OnPendingInputError != nil {
			s.OnPendingInputError(err)
		}
		return
	}
	for _, item := range items {
		if err := s.PendingInput.Enqueue(sessionID, item.Content); err != nil && s.OnPendingInputError != nil {
			s.OnPendingInputError(err)
		}
	}
}

func (s AgentLoopService) hasPendingInput(current *session.Session) bool {
	if s.PendingInput == nil || current == nil {
		return false
	}
	return s.PendingInput.HasPending(current.ID)
}

func (s AgentLoopService) hasImmediatePendingInput(current *session.Session) bool {
	if s.PendingInput == nil || current == nil {
		return false
	}
	if immediate, ok := s.PendingInput.(generationport.ImmediatePendingTurnInput); ok {
		return immediate.HasImmediatePending(current.ID)
	}
	// Legacy queues have no delivery mode metadata and historically represented
	// all pending input as a steer of the active turn.
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
