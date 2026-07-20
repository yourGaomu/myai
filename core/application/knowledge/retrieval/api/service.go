package api

import (
	"context"

	retrievalcommand "myai/core/application/knowledge/retrieval/command"
	retrievalresult "myai/core/application/knowledge/retrieval/result"
)

type Service interface {
	Retrieve(ctx context.Context, command retrievalcommand.Retrieve) (retrievalresult.Retrieve, error)
}
