package subagent

import (
	"context"

	domainsubagent "myai/core/domain/subagent"
	"myai/core/llm"
	modelport "myai/core/port/model"
)

type ScheduledTask func(ctx context.Context)

type ChildSessionRequest struct {
	SessionID          string
	ParentSessionID    string
	ParentTaskID       string
	Definition         domainsubagent.DefinitionSnapshot
	FallbackModelID    string
	WorkspaceRoot      string
	WorkspaceSandboxID string
}

type AgentRunRequest struct {
	SessionID   string
	Instruction string
	Title       string
	Stream      modelport.ChatStreamHandler
}

type AgentRunResult struct {
	Content   string
	Reasoning string
	Usage     llm.TokenUsage
}

type ParentContinuationRequest struct {
	Task domainsubagent.Task
	// 1. MessageID 贯穿 claim、入队和确认，保证父代理恢复使用同一个事件。
	MessageID string
	Stream    llm.ChatStreamHandler
}

type ParentContinuationResult struct {
	Content   string
	Reasoning string
	Usage     llm.TokenUsage
}
