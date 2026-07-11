package port

import (
	"context"

	generationcommand "myai/core/application/chat/generation/command"
	plancommand "myai/core/application/plan/command"
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
	Save(ctx context.Context, command plancommand.SaveState) (*agentplan.Plan, error)
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
