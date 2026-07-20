package knowledge

import (
	"context"

	domainknowledge "myai/core/domain/knowledge"
)

type VectorStore interface {
	EnsureIndex(ctx context.Context, definition domainknowledge.VectorIndexDefinition) error
	Upsert(ctx context.Context, embeddings []domainknowledge.EmbeddingVector) error
	Search(ctx context.Context, query domainknowledge.VectorQuery) ([]domainknowledge.VectorHit, error)
	MarkDeleted(ctx context.Context, deletion domainknowledge.VectorDeletion) error
	Health(ctx context.Context) error
}
