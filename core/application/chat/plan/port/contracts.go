package port

import (
	"context"

	generationcommand "myai/core/application/chat/generation/command"
	plancommand "myai/core/application/chat/plan/command"
	planstatecommand "myai/core/application/plan/command"
	messagecommand "myai/core/application/session/message/command"
	messageresult "myai/core/application/session/message/result"
	agentplan "myai/core/plan"
	"myai/core/session"
)

type SessionLoader interface {
	Load(ctx context.Context, sessionID string) (*session.Session, error)
}

type MessageAppender interface {
	AppendUserMessage(ctx context.Context, command messagecommand.AppendUserMessage) (messageresult.Command, error)
}

type StateStore interface {
	Save(ctx context.Context, command planstatecommand.SaveState) (*agentplan.Plan, error)
}

type UserMessagePersistence interface {
	PersistUserMessage(command generationcommand.PersistUserMessage)
}

type SessionEventPublisher interface {
	SessionChanged(ctx context.Context, sessionID string, reason string)
}

type UpdateSink interface {
	PlanUpdated(currentPlan *agentplan.Plan)
}

// RecoveryPlanner returns replacement work after a step has exhausted its
// bounded retries. It is intentionally optional so existing integrations can
// retain fail-fast behavior.
type RecoveryPlanner interface {
	Recover(ctx context.Context, request plancommand.RecoveryRequest) (*agentplan.Plan, error)
}
