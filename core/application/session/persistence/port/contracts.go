package port

import (
	"context"

	repository "myai/core/port/repository"
	"myai/core/session"
)

type SnapshotMemory interface {
	GetSession(sessionID string) (*session.Session, error)
}

type Repository interface {
	GetSession(ctx context.Context, sessionID string) (repository.SessionRecord, error)
	SaveSession(ctx context.Context, record repository.SessionRecord) error
}
