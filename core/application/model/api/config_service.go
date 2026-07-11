package api

import (
	"context"

	modelcommand "myai/core/application/model/command"
	modelresult "myai/core/application/model/result"
)

type ConfigService interface {
	AddConfig(ctx context.Context, command modelcommand.AddConfig) (modelresult.AddConfig, error)
}
