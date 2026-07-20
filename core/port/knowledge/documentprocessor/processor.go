package documentprocessor

import (
	"context"

	domainknowledge "myai/core/domain/knowledge"
)

type DocumentProcessor interface {
	Process(ctx context.Context, request Request, sink ChunkSink) (domainknowledge.ProcessingSummary, error)
}
