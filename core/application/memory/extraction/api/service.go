package api

import (
	"context"

	"myai/core/application/memory/extraction/command"
	"myai/core/application/memory/extraction/result"
)

type Service interface {
	EnqueueRun(ctx context.Context, command command.EnqueueRun) (result.Job, error)
	ListJobs(ctx context.Context, command command.ListJobs) (result.Jobs, error)
	Process(ctx context.Context, command command.Process) error
	Recover(ctx context.Context, command command.Recover) error
	Retry(ctx context.Context, command command.Retry) (result.Job, error)
}
