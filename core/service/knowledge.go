package service

import (
	"context"
	"errors"

	catalogapi "myai/core/application/knowledge/catalog/api"
	catalogcommand "myai/core/application/knowledge/catalog/command"
	catalogresult "myai/core/application/knowledge/catalog/result"
	documentapi "myai/core/application/knowledge/document/api"
	documentcommand "myai/core/application/knowledge/document/command"
	documentresult "myai/core/application/knowledge/document/result"
	queryapi "myai/core/application/knowledge/query/api"
	querycommand "myai/core/application/knowledge/query/command"
	queryresult "myai/core/application/knowledge/query/result"
	searchapi "myai/core/application/knowledge/search/api"
	searchcommand "myai/core/application/knowledge/search/command"
	searchresult "myai/core/application/knowledge/search/result"
)

type KnowledgeService struct {
	Catalog   catalogapi.Service
	Query     queryapi.Service
	Search    searchapi.Service
	Documents documentapi.Service
}

func (service *KnowledgeService) IngestDocument(ctx context.Context, command documentcommand.Ingest) (documentresult.Ingest, error) {
	if service == nil || service.Documents == nil {
		return documentresult.Ingest{}, errors.New("knowledge document service is not configured")
	}
	return service.Documents.Ingest(ctx, command)
}

func (service *KnowledgeService) RetryDocument(ctx context.Context, command documentcommand.Retry) (documentresult.Retry, error) {
	if service == nil || service.Documents == nil {
		return documentresult.Retry{}, errors.New("knowledge document service is not configured")
	}
	return service.Documents.Retry(ctx, command)
}

func (service *KnowledgeService) DeleteDocument(ctx context.Context, command documentcommand.Delete) error {
	if service == nil || service.Documents == nil {
		return errors.New("knowledge document service is not configured")
	}
	return service.Documents.Delete(ctx, command)
}

func (service *KnowledgeService) ListCatalog(ctx context.Context, includeDeleted bool) (catalogresult.Catalog, error) {
	if service == nil || service.Catalog == nil {
		return catalogresult.Catalog{}, errors.New("knowledge catalog service is not configured")
	}
	return service.Catalog.List(ctx, catalogcommand.List{IncludeDeleted: includeDeleted})
}

func (service *KnowledgeService) CreateCategory(ctx context.Context, command catalogcommand.CreateCategory) (catalogresult.Category, error) {
	if service == nil || service.Catalog == nil {
		return catalogresult.Category{}, errors.New("knowledge catalog service is not configured")
	}
	return service.Catalog.CreateCategory(ctx, command)
}

func (service *KnowledgeService) MoveCategory(ctx context.Context, command catalogcommand.MoveCategory) (catalogresult.Category, error) {
	if service == nil || service.Catalog == nil {
		return catalogresult.Category{}, errors.New("knowledge catalog service is not configured")
	}
	return service.Catalog.MoveCategory(ctx, command)
}

func (service *KnowledgeService) DeleteCategory(ctx context.Context, command catalogcommand.DeleteCategory) error {
	if service == nil || service.Catalog == nil {
		return errors.New("knowledge catalog service is not configured")
	}
	return service.Catalog.DeleteCategory(ctx, command)
}

func (service *KnowledgeService) CreateKnowledgeBase(ctx context.Context, command catalogcommand.CreateKnowledgeBase) (catalogresult.KnowledgeBase, error) {
	if service == nil || service.Catalog == nil {
		return catalogresult.KnowledgeBase{}, errors.New("knowledge catalog service is not configured")
	}
	return service.Catalog.CreateKnowledgeBase(ctx, command)
}

func (service *KnowledgeService) UpdateKnowledgeBase(ctx context.Context, command catalogcommand.UpdateKnowledgeBase) (catalogresult.KnowledgeBase, error) {
	if service == nil || service.Catalog == nil {
		return catalogresult.KnowledgeBase{}, errors.New("knowledge catalog service is not configured")
	}
	return service.Catalog.UpdateKnowledgeBase(ctx, command)
}

func (service *KnowledgeService) DeleteKnowledgeBase(ctx context.Context, command catalogcommand.DeleteKnowledgeBase) error {
	if service == nil || service.Catalog == nil {
		return errors.New("knowledge catalog service is not configured")
	}
	return service.Catalog.DeleteKnowledgeBase(ctx, command)
}

func (service *KnowledgeService) ListDocuments(ctx context.Context, command querycommand.Documents) (queryresult.Documents, error) {
	if service == nil || service.Query == nil {
		return queryresult.Documents{}, errors.New("knowledge query service is not configured")
	}
	return service.Query.Documents(ctx, command)
}

func (service *KnowledgeService) ListIndexProfiles(ctx context.Context, includeDeleted bool) (queryresult.IndexProfiles, error) {
	if service == nil || service.Query == nil {
		return queryresult.IndexProfiles{}, errors.New("knowledge query service is not configured")
	}
	return service.Query.IndexProfiles(ctx, querycommand.IndexProfiles{IncludeDeleted: includeDeleted})
}

func (service *KnowledgeService) SearchKnowledge(ctx context.Context, command searchcommand.Search) (searchresult.Search, error) {
	if service == nil || service.Search == nil {
		return searchresult.Search{}, errors.New("knowledge search service is not configured")
	}
	return service.Search.Search(ctx, command)
}
