package port

import (
	"context"

	sessioncommand "myai/core/application/session/command"
	repository "myai/core/port/repository"
)

type SessionPersistence interface {
	Save(ctx context.Context, command sessioncommand.SaveSession) error
	PrepareRecord(ctx context.Context, record repository.SessionRecord) (repository.SessionRecord, error)
	SaveRecord(ctx context.Context, record repository.SessionRecord) error
}
