package port

import (
	"context"

	repository "myai/core/port/repository"
)

type MessageSaver interface {
	SaveMessage(ctx context.Context, message repository.MessageRecord) error
}
