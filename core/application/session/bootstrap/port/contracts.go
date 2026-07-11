package port

import (
	"context"

	sessioncommand "myai/core/application/session/command"
	"myai/core/session"
)

type Cache interface {
	Get(ctx context.Context) (string, error)
	Save(ctx context.Context, sessionID string) error
}

type Lifecycle interface {
	NewSession(ctx context.Context) (*session.Session, error)
	LoadSession(ctx context.Context, sessionID string) (*session.Session, error)
}

type State interface {
	CurrentSession() (*session.Session, error)
}

type Persistence interface {
	Save(ctx context.Context, command sessioncommand.SaveSession) error
}
