package api

import (
	"context"

	sessioncommand "myai/core/application/session/command"
	repository "myai/core/port/repository"
)

type Service interface {
	Save(ctx context.Context, command sessioncommand.SaveSession) error
	SaveRecord(ctx context.Context, record repository.SessionRecord) error
}
