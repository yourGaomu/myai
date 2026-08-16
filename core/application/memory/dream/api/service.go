package api

import (
	"context"

	"myai/core/application/memory/dream/command"
	"myai/core/application/memory/dream/result"
)

type Service interface {
	Run(ctx context.Context, command command.Run) (result.Run, error)
	Get(ctx context.Context, command command.Get) (result.Run, error)
	List(ctx context.Context, command command.List) (result.List, error)
}
