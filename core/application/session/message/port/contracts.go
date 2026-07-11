package port

import (
	"context"

	"myai/core/session"
)

type SessionLoader interface {
	Load(ctx context.Context, sessionID string) (*session.Session, error)
}

type CommandMemory interface {
	CurrentSessionId() string
	AddUserMessageTo(sessionID string, input string) error
	TrimAfterLastUserMessage(sessionID string) (string, error)
	GetSession(sessionID string) (*session.Session, error)
}
