package port

import (
	"context"

	domainmessage "myai/core/domain/message"
	"myai/core/llm"
	agentplan "myai/core/plan"
	repository "myai/core/port/repository"
	"myai/core/session"
)

type MemoryStore interface {
	GetSession(sessionID string) (*session.Session, error)
	UseSession(sessionID string) error
	PutSessionWithModeUsage(sessionID string, modelID string, agentMode session.AgentMode, permissionMode session.PermissionMode, contextWindowK int, summary string, compactedMessages int, usage llm.TokenUsage, lastUsage llm.TokenUsage, messages []domainmessage.Message) error
	PutSessionWithModeUsageNoCurrent(sessionID string, modelID string, agentMode session.AgentMode, permissionMode session.PermissionMode, contextWindowK int, summary string, compactedMessages int, usage llm.TokenUsage, lastUsage llm.TokenUsage, messages []domainmessage.Message) error
	SetCurrentPlanForSession(sessionID string, currentPlan *agentplan.Plan) error
}

type SessionRecordGetter interface {
	GetSession(ctx context.Context, sessionID string) (repository.SessionRecord, error)
}

type MessageRecordLister interface {
	ListMessages(ctx context.Context, sessionID string) ([]repository.MessageRecord, error)
}
