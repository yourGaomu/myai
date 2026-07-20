package api

import (
	"context"

	documentcommand "myai/core/application/knowledge/document/command"
	documentresult "myai/core/application/knowledge/document/result"
)

type Service interface {
	Ingest(ctx context.Context, command documentcommand.Ingest) (documentresult.Ingest, error)
	Retry(ctx context.Context, command documentcommand.Retry) (documentresult.Retry, error)
	Delete(ctx context.Context, command documentcommand.Delete) error
}
