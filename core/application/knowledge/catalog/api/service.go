package api

import (
	"context"

	catalogcommand "myai/core/application/knowledge/catalog/command"
	catalogresult "myai/core/application/knowledge/catalog/result"
)

type Service interface {
	List(ctx context.Context, command catalogcommand.List) (catalogresult.Catalog, error)
	CreateCategory(ctx context.Context, command catalogcommand.CreateCategory) (catalogresult.Category, error)
	MoveCategory(ctx context.Context, command catalogcommand.MoveCategory) (catalogresult.Category, error)
	DeleteCategory(ctx context.Context, command catalogcommand.DeleteCategory) error
	CreateKnowledgeBase(ctx context.Context, command catalogcommand.CreateKnowledgeBase) (catalogresult.KnowledgeBase, error)
	UpdateKnowledgeBase(ctx context.Context, command catalogcommand.UpdateKnowledgeBase) (catalogresult.KnowledgeBase, error)
	DeleteKnowledgeBase(ctx context.Context, command catalogcommand.DeleteKnowledgeBase) error
}
