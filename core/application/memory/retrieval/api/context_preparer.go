package api

import (
	"context"

	"myai/core/application/memory/retrieval/command"
	"myai/core/application/memory/retrieval/result"
)

type ContextPreparer interface {
	Prepare(ctx context.Context, command command.Prepare) (result.Context, error)
}
