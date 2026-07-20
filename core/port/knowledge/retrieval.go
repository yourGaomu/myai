package knowledge

import (
	"context"

	domainknowledge "myai/core/domain/knowledge"
)

type Reranker interface {
	Rerank(ctx context.Context, query domainknowledge.RetrievalQuery, hits []domainknowledge.RetrievalHit) ([]domainknowledge.RetrievalHit, error)
}

type RetrievalCache interface {
	Store(ctx context.Context, embeddings []domainknowledge.EmbeddingVector, chunks []domainknowledge.Chunk) error
	Evict(ctx context.Context, targetBytes int64) error
	Health(ctx context.Context) error
}
