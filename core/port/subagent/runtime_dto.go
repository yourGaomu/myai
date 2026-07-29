package subagent

import (
	"context"

	domainsubagent "myai/core/domain/subagent"
	"myai/core/llm"
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
}

type AgentRunResult struct {
	Content   string
	Reasoning string
	Usage     llm.TokenUsage
}

type ParentContinuationRequest struct {
	Task   domainsubagent.Task
	Stream llm.ChatStreamHandler
}

type ParentContinuationResult struct {
	Content   string
	Reasoning string
	Usage     llm.TokenUsage
}
