package api

import (
	"context"

	indexingcommand "myai/core/application/knowledge/indexing/command"
	indexingresult "myai/core/application/knowledge/indexing/result"
)

type Service interface {
	Submit(ctx context.Context, command indexingcommand.Submit) (indexingresult.Submit, error)
	Run(ctx context.Context, command indexingcommand.Run) (indexingresult.Run, error)
}
