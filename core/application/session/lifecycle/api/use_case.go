package api

import (
	"context"

	lifecyclecommand "myai/core/application/session/lifecycle/command"
	lifecycleresult "myai/core/application/session/lifecycle/result"
)

type UseCase interface {
	Create(ctx context.Context, command lifecyclecommand.CreateSession) (lifecycleresult.Lifecycle, error)
	Load(ctx context.Context, command lifecyclecommand.LoadSession) (lifecycleresult.Lifecycle, error)
	Delete(ctx context.Context, command lifecyclecommand.DeleteSession) (lifecycleresult.Lifecycle, error)
	Restore(ctx context.Context, command lifecyclecommand.RestoreSession) (lifecycleresult.Lifecycle, error)
	Clear(ctx context.Context, command lifecyclecommand.ClearSession) (lifecycleresult.Lifecycle, error)
}
