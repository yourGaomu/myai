package knowledge

import (
	"context"
	"time"

	domainknowledge "myai/core/domain/knowledge"
)

type KnowledgeBaseRepository interface {
	Get(ctx context.Context, knowledgeBaseID string) (domainknowledge.KnowledgeBase, error)
	List(ctx context.Context, includeDeleted bool) ([]domainknowledge.KnowledgeBase, error)
	Save(ctx context.Context, knowledgeBase domainknowledge.KnowledgeBase) error
	MarkDeleted(ctx context.Context, knowledgeBaseID string, deletedAt time.Time, reason string, syncSequence int64) error
}

type KnowledgeCategoryRepository interface {
	Get(ctx context.Context, categoryID string) (domainknowledge.KnowledgeCategory, error)
	List(ctx context.Context, includeDeleted bool) ([]domainknowledge.KnowledgeCategory, error)
	Save(ctx context.Context, category domainknowledge.KnowledgeCategory) error
	MarkDeleted(ctx context.Context, categoryID string, deletedAt time.Time, reason string) error
}

type DocumentRepository interface {
	Get(ctx context.Context, documentID string) (domainknowledge.Document, error)
	ListByKnowledgeBase(ctx context.Context, knowledgeBaseID string, includeDeleted bool) ([]domainknowledge.Document, error)
	Save(ctx context.Context, document domainknowledge.Document) error
	MarkDeleted(ctx context.Context, documentID string, deletedAt time.Time, reason string, syncSequence int64) error
}

type ChunkRepository interface {
	SaveAll(ctx context.Context, chunks []domainknowledge.Chunk) error
	GetByIDs(ctx context.Context, chunkIDs []string) ([]domainknowledge.Chunk, error)
	ListByDocumentPage(ctx context.Context, documentID string, documentVersion int64, parsingProfileID string, chunkingProfileID string, afterOrdinal int, limit int) ([]domainknowledge.Chunk, error)
	ListMissingEmbeddings(ctx context.Context, parsingProfileID string, chunkingProfileID string, embeddingProfileID string, limit int) ([]domainknowledge.Chunk, error)
	MarkEmbedded(ctx context.Context, chunkIDs []string, embeddingProfileID string, updatedAt time.Time) error
	MarkDeletedByDocument(ctx context.Context, documentID string, deletedAt time.Time, syncSequence int64) error
}

type ProfileRepository interface {
	GetParsingProfile(ctx context.Context, profileID string) (domainknowledge.ParsingProfile, error)
	SaveParsingProfile(ctx context.Context, profile domainknowledge.ParsingProfile) error
	GetEmbeddingProfile(ctx context.Context, profileID string) (domainknowledge.EmbeddingProfile, error)
	SaveEmbeddingProfile(ctx context.Context, profile domainknowledge.EmbeddingProfile) error
	GetChunkingProfile(ctx context.Context, profileID string) (domainknowledge.ChunkingProfile, error)
	SaveChunkingProfile(ctx context.Context, profile domainknowledge.ChunkingProfile) error
	GetIndexProfile(ctx context.Context, profileID string) (domainknowledge.IndexProfile, error)
	SaveIndexProfile(ctx context.Context, profile domainknowledge.IndexProfile) error
	MarkParsingProfileDeleted(ctx context.Context, profileID string, deletedAt time.Time, reason string) error
	MarkEmbeddingProfileDeleted(ctx context.Context, profileID string, deletedAt time.Time, reason string) error
	MarkChunkingProfileDeleted(ctx context.Context, profileID string, deletedAt time.Time, reason string) error
	MarkIndexProfileDeleted(ctx context.Context, profileID string, deletedAt time.Time, reason string) error
}

type IndexingJobRepository interface {
	Get(ctx context.Context, jobID string) (domainknowledge.IndexingJob, error)
	Save(ctx context.Context, job domainknowledge.IndexingJob) error
	ListPending(ctx context.Context, limit int) ([]domainknowledge.IndexingJob, error)
}

// IndexingStateRepository persists the document and its indexing job as one
// state transition. Implementations must not commit only one side.
type IndexingStateRepository interface {
	SaveDocumentAndJob(ctx context.Context, document domainknowledge.Document, job domainknowledge.IndexingJob) error
}

type IndexingJobQueryRepository interface {
	ListByKnowledgeBase(ctx context.Context, knowledgeBaseID string, limit int) ([]domainknowledge.IndexingJob, error)
}

type IndexProfileCatalog interface {
	ListIndexProfiles(ctx context.Context, includeDeleted bool) ([]domainknowledge.IndexProfile, error)
}

type SyncChangeRepository interface {
	Append(ctx context.Context, change domainknowledge.SyncChange) error
	ListAfter(ctx context.Context, sequence int64, limit int) ([]domainknowledge.SyncChange, error)
}
