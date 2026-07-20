package repository

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	knowledgemapper "myai/core/adapter/persistence/mongo/knowledge/mapper"
	"myai/core/adapter/persistence/mongo/knowledge/po"
	mongotemplate "myai/core/adapter/persistence/mongo/template"
	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

type KnowledgeCategoryRepository struct {
	operations mongotemplate.Operations
}

var _ knowledgeport.KnowledgeCategoryRepository = (*KnowledgeCategoryRepository)(nil)

func NewKnowledgeCategoryRepository(client *gomongo.Client, database string) *KnowledgeCategoryRepository {
	if client == nil || database == "" {
		return NewKnowledgeCategoryRepositoryWithOperations(mongotemplate.New(nil))
	}
	return NewKnowledgeCategoryRepositoryWithOperations(mongotemplate.New(client.Database(database)))
}

func NewKnowledgeCategoryRepositoryWithOperations(operations mongotemplate.Operations) *KnowledgeCategoryRepository {
	if operations == nil {
		operations = mongotemplate.New(nil)
	}
	return &KnowledgeCategoryRepository{operations: operations}
}

func (repository *KnowledgeCategoryRepository) Get(ctx context.Context, categoryID string) (domainknowledge.KnowledgeCategory, error) {
	var document po.KnowledgeCategoryDocument
	if err := repository.operations.FindOne(ctx, knowledgeCategoriesCollection, activeIDFilter(categoryID), &document); err != nil {
		return domainknowledge.KnowledgeCategory{}, repositoryError(err)
	}
	return knowledgemapper.KnowledgeCategoryDomainFromDocument(document), nil
}

func (repository *KnowledgeCategoryRepository) List(ctx context.Context, includeDeleted bool) ([]domainknowledge.KnowledgeCategory, error) {
	filter := activeFilter()
	if includeDeleted {
		filter = bson.M{}
	}
	var documents []po.KnowledgeCategoryDocument
	if err := repository.operations.FindAll(ctx, knowledgeCategoriesCollection, filter, &documents, options.Find().SetSort(bson.D{{Key: "sort_order", Value: 1}, {Key: "name", Value: 1}})); err != nil {
		return nil, err
	}
	categories := make([]domainknowledge.KnowledgeCategory, 0, len(documents))
	for _, document := range documents {
		categories = append(categories, knowledgemapper.KnowledgeCategoryDomainFromDocument(document))
	}
	return categories, nil
}

func (repository *KnowledgeCategoryRepository) Save(ctx context.Context, category domainknowledge.KnowledgeCategory) error {
	if err := category.Validate(); err != nil {
		return fmt.Errorf("validate knowledge category: %w", err)
	}
	if category.Deletion.Deleted {
		return fmt.Errorf("save cannot persist a deleted knowledge category; use MarkDeleted")
	}
	document := knowledgemapper.KnowledgeCategoryDocumentFromDomain(category)
	_, err := repository.operations.UpdateOne(ctx, knowledgeCategoriesCollection, activeIDFilter(document.ID), bson.M{
		"$set": bson.M{
			"name":         document.Name,
			"parent_id":    document.ParentID,
			"ancestor_ids": document.AncestorIDs,
			"sort_order":   document.SortOrder,
			"updated_at":   document.UpdatedAt,
		},
		"$setOnInsert": bson.M{
			"_id":        document.ID,
			"deleted":    false,
			"created_at": document.CreatedAt,
		},
	}, options.UpdateOne().SetUpsert(true))
	return err
}

func (repository *KnowledgeCategoryRepository) MarkDeleted(ctx context.Context, categoryID string, deletedAt time.Time, reason string) error {
	result, err := repository.operations.UpdateOne(ctx, knowledgeCategoriesCollection, bson.M{"_id": categoryID}, bson.M{"$set": bson.M{
		"deleted":       true,
		"deleted_at":    deletedAt,
		"delete_reason": reason,
		"updated_at":    deletedAt,
	}})
	return matchedOrNotFound(result, err)
}
