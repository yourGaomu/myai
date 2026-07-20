package api

import (
	"context"

	retrievalcommand "myai/core/application/chat/retrieval/command"
	retrievalresult "myai/core/application/chat/retrieval/result"
)

type ContextPreparer interface {
	Prepare(ctx context.Context, command retrievalcommand.Prepare) (retrievalresult.Context, error)
}
