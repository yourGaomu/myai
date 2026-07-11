package api

import (
	"context"

	loadcommand "myai/core/application/session/load/command"
	"myai/core/session"
)

type Service interface {
	Load(ctx context.Context, sessionID string) (*session.Session, error)
	LoadCurrent(ctx context.Context, sessionID string) (*session.Session, error)
	EnsureInMemory(ctx context.Context, command loadcommand.EnsureInMemory) (*session.Session, error)
}
