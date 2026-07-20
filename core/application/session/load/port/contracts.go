package port

import (
	"context"

	agentplan "myai/core/plan"
	repository "myai/core/port/repository"
	"myai/core/session"
)

type MemoryStore interface {
	GetSession(sessionID string) (*session.Session, error)
	UseSession(sessionID string) error
	PutSessionState(state session.InitialState, setCurrent bool) error
	SetCurrentPlanForSession(sessionID string, currentPlan *agentplan.Plan) error
}

type SessionRecordGetter interface {
	GetSession(ctx context.Context, sessionID string) (repository.SessionRecord, error)
}

type MessageRecordLister interface {
	ListMessages(ctx context.Context, sessionID string) ([]repository.MessageRecord, error)
}
