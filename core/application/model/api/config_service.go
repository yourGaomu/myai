package api

import (
	"context"

	modelcommand "myai/core/application/model/command"
	modelresult "myai/core/application/model/result"
)

type ConfigService interface {
	AddConfig(ctx context.Context, command modelcommand.AddConfig) (modelresult.AddConfig, error)
	UpdateConfig(ctx context.Context, command modelcommand.UpdateConfig) (modelresult.ConfigMutation, error)
	SetEnabled(ctx context.Context, command modelcommand.SetEnabled) (modelresult.ConfigMutation, error)
	SetDefault(ctx context.Context, command modelcommand.SetDefault) (modelresult.ConfigMutation, error)
	DeleteConfig(ctx context.Context, command modelcommand.DeleteConfig) (modelresult.ConfigMutation, error)
	TestConfig(ctx context.Context, command modelcommand.AddConfig) (modelresult.TestConfig, error)
}
