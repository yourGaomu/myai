package api

import (
	"context"

	currentresult "myai/core/application/session/current/result"
	"myai/core/session"
)

type SessionService interface {
	Get(ctx context.Context) (string, error)
	Save(ctx context.Context, sessionID string) error
	Delete(ctx context.Context) error
}

type StateQueryService interface {
	State() currentresult.State
	CurrentSession() (*session.Session, error)
}
