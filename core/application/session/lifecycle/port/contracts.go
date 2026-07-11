package port

import (
	"context"
	"time"

	sessioncommand "myai/core/application/session/command"
	lifecyclecommand "myai/core/application/session/lifecycle/command"
	lifecycleresult "myai/core/application/session/lifecycle/result"
	sessionresult "myai/core/application/session/result"
	repository "myai/core/port/repository"
	"myai/core/session"
)

type MemoryStore interface {
	NewSession() error
	Current() (*session.Session, error)
	CurrentSessionId() string
	ClearCurrent() error
	RemoveSession(sessionID string) error
	GetSession(sessionID string) (*session.Session, error)
	UseSession(sessionID string) error
}

type SessionLoader interface {
	LoadCurrent(ctx context.Context, sessionID string) (*session.Session, error)
}

type SessionRepository interface {
	GetSession(ctx context.Context, sessionID string) (repository.SessionRecord, error)
	MarkSessionDeleted(ctx context.Context, sessionID string, deletedAt time.Time) error
	MarkSessionRestored(ctx context.Context, sessionID string, restoredAt time.Time) error
}

type MessageRepository interface {
	ClearMessages(ctx context.Context, sessionID string) error
}

type Persistence interface {
	Save(ctx context.Context, command sessioncommand.SaveSession) error
}

type CurrentSession interface {
	Save(ctx context.Context, sessionID string) error
}

type SessionQuery interface {
	ListSessions(ctx context.Context, includeDeleted bool) ([]sessionresult.SessionListItem, error)
}

type EventPublisher interface {
	SessionChanged(ctx context.Context, sessionID string, reason string)
}

type LifecycleService interface {
	NewSession(ctx context.Context) (*session.Session, error)
	LoadSession(ctx context.Context, sessionID string) (*session.Session, error)
	DeleteSession(ctx context.Context, command lifecyclecommand.DeleteSession) (lifecycleresult.DeleteSession, error)
	RestoreSession(ctx context.Context, command lifecyclecommand.RestoreSession) (string, error)
	ClearCurrent(ctx context.Context) (*session.Session, error)
}
