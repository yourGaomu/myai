package chat

import (
	"context"
	"errors"
	"time"

	domainmessage "myai/core/domain/message"
	domainsubagent "myai/core/domain/subagent"
	"myai/core/llm"
	subagentport "myai/core/port/subagent"
	"myai/core/service"
)

type Continuation struct {
	Chat *service.ChatService
}

var _ subagentport.ParentContinuation = Continuation{}
var _ subagentport.ParentCompletionNotifier = Continuation{}
var _ subagentport.PendingParentContinuation = Continuation{}

// Kept as package-level aliases for the adapter tests; the canonical limits
// and formatter live in the subagent domain package.
const (
	maxContinuationResultRunes  = 12000
	maxContinuationErrorRunes   = 2000
	maxContinuationChangedFiles = 200
	maxContinuationPathRunes    = 512
)

func (continuation Continuation) Continue(ctx context.Context, request subagentport.ParentContinuationRequest) (subagentport.ParentContinuationResult, error) {
	if continuation.Chat == nil {
		return subagentport.ParentContinuationResult{}, errors.New("subagent parent chat continuation is nil")
	}
	prompt, err := continuationPrompt(request)
	if err != nil {
		return subagentport.ParentContinuationResult{}, err
	}
	kind := domainsubagent.AgentMessageKindTaskResult
	// 1. 根据子代理终态选择 result/error 消息类型，避免失败被当成成功结果处理。
	if request.Task.Status == domainsubagent.TaskStatusFailed {
		kind = domainsubagent.AgentMessageKindTaskError
	}
	response, err := continuation.Chat.ContinueSessionStreamForMessage(
		ctx,
		request.Task.ParentSessionID,
		prompt,
		domainmessage.SyntheticReasonSubagentResult,
		request.MessageID,
		string(kind),
		request.Stream,
	)
	if err != nil {
		return subagentport.ParentContinuationResult{}, err
	}
	return subagentport.ParentContinuationResult{
		Content: response.Result.Content, Reasoning: response.Result.Reasoning, Usage: response.Result.Usage,
	}, nil
}

func (continuation Continuation) Notify(ctx context.Context, task domainsubagent.Task) error {
	if continuation.Chat == nil {
		return errors.New("subagent parent chat continuation is nil")
	}
	prompt, err := continuationPrompt(subagentport.ParentContinuationRequest{Task: task})
	if err != nil {
		return err
	}
	// 2. 优先把完整 AgentMessage 放入结构化 mailbox，保留任务来源和投递语义。
	messageKind := domainsubagent.AgentMessageKindTaskResult
	if task.Status == domainsubagent.TaskStatusFailed {
		messageKind = domainsubagent.AgentMessageKindTaskError
	}
	message := domainsubagent.AgentMessage{
		ID: domainsubagent.AgentResultMessageID(task.ID), SourceTaskID: task.ID,
		AuthorAgentID: task.ChildSessionID, RecipientAgentID: task.ParentSessionID,
		ParentTurnID: task.ParentRunID, RootAgentID: task.ParentSessionID,
		// 子代理完成后要求父 Agent 在空闲时自动开启 continuation；如果父 Agent
		// 仍在运行，AgentLoop 会在回答边界按 queue 语义安全领取这条消息。
		Kind: messageKind, Content: prompt, Trigger: domainsubagent.AgentMessageTriggerTurn,
		Status: domainsubagent.AgentMessagePending, CreatedAt: time.Now().UTC(),
	}
	if err := message.Validate(); err != nil {
		return err
	}
	return continuation.Chat.EnqueueTurnInputAgentMessage(task.ParentSessionID, message)
}

func (continuation Continuation) ContinuePending(ctx context.Context, sessionID string, stream llm.ChatStreamHandler) (subagentport.ParentContinuationResult, error) {
	if continuation.Chat == nil {
		return subagentport.ParentContinuationResult{}, errors.New("subagent parent chat continuation is nil")
	}
	response, err := continuation.Chat.ContinuePendingStreamForSession(ctx, sessionID, stream)
	if err != nil {
		return subagentport.ParentContinuationResult{}, err
	}
	return subagentport.ParentContinuationResult{
		Content: response.Result.Content, Reasoning: response.Result.Reasoning, Usage: response.Result.Usage,
	}, nil
}

func continuationPrompt(request subagentport.ParentContinuationRequest) (string, error) {
	return domainsubagent.CompletionMessageContent(request.Task)
}
