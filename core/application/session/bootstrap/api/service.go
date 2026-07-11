package api

import (
	"context"

	bootstrapcommand "myai/core/application/session/bootstrap/command"
	bootstrapresult "myai/core/application/session/bootstrap/result"
)

type Service interface {
	Bootstrap(ctx context.Context, command bootstrapcommand.Bootstrap) (bootstrapresult.Bootstrap, error)
}
