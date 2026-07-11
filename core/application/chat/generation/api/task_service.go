package api

import (
	"context"

	generationcommand "myai/core/application/chat/generation/command"
	generationresult "myai/core/application/chat/generation/result"
)

type TaskService interface {
	Generate(ctx context.Context, command generationcommand.GenerationTask) (generationresult.GenerationResponse, error)
}
