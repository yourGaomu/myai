package api

import (
	"context"

	modelcommand "myai/core/application/model/command"
	modelresult "myai/core/application/model/result"
)

type BootstrapService interface {
	Bootstrap(ctx context.Context, command modelcommand.Bootstrap) (modelresult.Bootstrap, error)
}
