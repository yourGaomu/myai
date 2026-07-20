package agent

import (
	"context"

	catalogcommand "myai/core/application/knowledge/catalog/command"
	catalogresult "myai/core/application/knowledge/catalog/result"
	documentcommand "myai/core/application/knowledge/document/command"
	documentresult "myai/core/application/knowledge/document/result"
	querycommand "myai/core/application/knowledge/query/command"
	queryresult "myai/core/application/knowledge/query/result"
	searchcommand "myai/core/application/knowledge/search/command"
	searchresult "myai/core/application/knowledge/search/result"
)

type KnowledgeFacade interface {
	ListCatalog(ctx context.Context, includeDeleted bool) (catalogresult.Catalog, error)
	CreateCategory(ctx context.Context, command catalogcommand.CreateCategory) (catalogresult.Category, error)
	MoveCategory(ctx context.Context, command catalogcommand.MoveCategory) (catalogresult.Category, error)
	DeleteCategory(ctx context.Context, command catalogcommand.DeleteCategory) error
	CreateKnowledgeBase(ctx context.Context, command catalogcommand.CreateKnowledgeBase) (catalogresult.KnowledgeBase, error)
	UpdateKnowledgeBase(ctx context.Context, command catalogcommand.UpdateKnowledgeBase) (catalogresult.KnowledgeBase, error)
	DeleteKnowledgeBase(ctx context.Context, command catalogcommand.DeleteKnowledgeBase) error
	ListDocuments(ctx context.Context, command querycommand.Documents) (queryresult.Documents, error)
	ListIndexProfiles(ctx context.Context, includeDeleted bool) (queryresult.IndexProfiles, error)
	SearchKnowledge(ctx context.Context, command searchcommand.Search) (searchresult.Search, error)
	IngestDocument(ctx context.Context, command documentcommand.Ingest) (documentresult.Ingest, error)
	RetryDocument(ctx context.Context, command documentcommand.Retry) (documentresult.Retry, error)
	DeleteDocument(ctx context.Context, command documentcommand.Delete) error
}
