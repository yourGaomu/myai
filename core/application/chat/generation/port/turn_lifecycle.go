package port

import (
	"context"

	generationcommand "myai/core/application/chat/generation/command"
)

type TurnLifecycleHooks interface {
	Handle(ctx context.Context, command generationcommand.TurnHook) (generationcommand.TurnHookOutcome, error)
}
