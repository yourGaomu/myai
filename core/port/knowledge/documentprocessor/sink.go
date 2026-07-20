package documentprocessor

import (
	"context"

	domainknowledge "myai/core/domain/knowledge"
)

type ChunkSink interface {
	Accept(ctx context.Context, chunks []domainknowledge.ChunkDraft) error
}
