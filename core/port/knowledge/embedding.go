package knowledge

import (
	"context"

	domainknowledge "myai/core/domain/knowledge"
)

type EmbeddingProvider interface {
	EmbedDocuments(ctx context.Context, request domainknowledge.EmbedRequest) (domainknowledge.EmbedResult, error)
	EmbedQuery(ctx context.Context, request domainknowledge.EmbedRequest) (domainknowledge.EmbedResult, error)
}

type EmbeddingModelRegistry interface {
	Get(modelID string) (EmbeddingProvider, bool)
	GetInfo(modelID string) (domainknowledge.EmbeddingModelInfo, bool)
	List() []domainknowledge.EmbeddingModelInfo
}

type MutableEmbeddingModelRegistry interface {
	EmbeddingModelRegistry
	Set(modelID string, provider EmbeddingProvider, info domainknowledge.EmbeddingModelInfo) error
}

type EmbeddingModelResolver interface {
	Resolve(profile domainknowledge.EmbeddingProfile) (EmbeddingProvider, error)
}
