package repository

import (
	"context"
	"time"
)

type SessionRepository interface {
	GetSession(ctx context.Context, sessionID string) (SessionRecord, error)
	SaveSession(ctx context.Context, session SessionRecord) error
	MarkSessionDeleted(ctx context.Context, sessionID string, deletedAt time.Time) error
	MarkSessionRestored(ctx context.Context, sessionID string, restoredAt time.Time) error
	ListSessions(ctx context.Context) ([]SessionRecord, error)
	ListSessionsWithDeleted(ctx context.Context, includeDeleted bool) ([]SessionRecord, error)
}
