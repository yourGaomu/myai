package api

import (
	"context"

	generationcommand "myai/core/application/chat/generation/command"
	generationresult "myai/core/application/chat/generation/result"
)

type Generator interface {
	Generate(ctx context.Context, command generationcommand.AssistantGeneration) (generationresult.GenerationResponse, error)
}
