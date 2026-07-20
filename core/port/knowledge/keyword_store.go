package knowledge

import (
	"context"

	domainknowledge "myai/core/domain/knowledge"
)

type KeywordStore interface {
	Upsert(ctx context.Context, documents []domainknowledge.KeywordDocument) error
	Search(ctx context.Context, query domainknowledge.KeywordQuery) ([]domainknowledge.KeywordHit, error)
	MarkDeleted(ctx context.Context, chunkIDs []string, syncSequence int64) error
}
