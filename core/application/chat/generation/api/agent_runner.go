package api

import (
	"context"

	generationcommand "myai/core/application/chat/generation/command"
	modelport "myai/core/port/model"
)

type AgentRunner interface {
	Run(ctx context.Context, command generationcommand.Run) (modelport.ChatResult, error)
}
