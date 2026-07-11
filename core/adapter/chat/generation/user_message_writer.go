package generation

import (
	"context"

	generationcommand "myai/core/application/chat/generation/command"
)

type UserMessageWriter interface {
	SaveUserMessage(ctx context.Context, command generationcommand.PersistUserMessage) error
}
