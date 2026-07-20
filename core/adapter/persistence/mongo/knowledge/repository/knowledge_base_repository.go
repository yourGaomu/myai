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

type KnowledgeBaseRepository struct {
	operations mongotemplate.Operations
}

var _ knowledgeport.KnowledgeBaseRepository = (*KnowledgeBaseRepository)(nil)

func NewKnowledgeBaseRepository(client *gomongo.Client, database string) *KnowledgeBaseRepository {
	if client == nil || database == "" {
		return NewKnowledgeBaseRepositoryWithOperations(mongotemplate.New(nil))
	}
	return NewKnowledgeBaseRepositoryWithOperations(mongotemplate.New(client.Database(database)))
}

func NewKnowledgeBaseRepositoryWithOperations(operations mongotemplate.Operations) *KnowledgeBaseRepository {
	if operations == nil {
		operations = mongotemplate.New(nil)
	}
	return &KnowledgeBaseRepository{operations: operations}
}

func (repository *KnowledgeBaseRepository) Get(ctx context.Context, knowledgeBaseID string) (domainknowledge.KnowledgeBase, error) {
	var document po.KnowledgeBaseDocument
	if err := repository.operations.FindOne(ctx, knowledgeBasesCollection, activeIDFilter(knowledgeBaseID), &document); err != nil {
		return domainknowledge.KnowledgeBase{}, repositoryError(err)
	}
	return knowledgemapper.KnowledgeBaseDomainFromDocument(document), nil
}

func (repository *KnowledgeBaseRepository) List(ctx context.Context, includeDeleted bool) ([]domainknowledge.KnowledgeBase, error) {
	filter := activeFilter()
	if includeDeleted {
		filter = bson.M{}
	}

	var documents []po.KnowledgeBaseDocument
	if err := repository.operations.FindAll(
		ctx,
		knowledgeBasesCollection,
		filter,
		&documents,
		options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}),
	); err != nil {
		return nil, err
	}
	bases := make([]domainknowledge.KnowledgeBase, 0, len(documents))
	for _, document := range documents {
		bases = append(bases, knowledgemapper.KnowledgeBaseDomainFromDocument(document))
	}
	return bases, nil
}

func (repository *KnowledgeBaseRepository) Save(ctx context.Context, base domainknowledge.KnowledgeBase) error {
	if err := base.Validate(); err != nil {
		return fmt.Errorf("validate knowledge base: %w", err)
	}
	if base.Deletion.Deleted {
		return fmt.Errorf("save cannot persist a deleted knowledge base; use MarkDeleted")
	}

	document := knowledgemapper.KnowledgeBaseDocumentFromDomain(base)
	setValues := bson.M{
		"category_id":              document.CategoryID,
		"name":                     document.Name,
		"description":              document.Description,
		"rag_enabled":              document.RAGEnabled,
		"active_index_profile_id":  document.ActiveIndexProfileID,
		"pending_index_profile_id": document.PendingIndexProfileID,
		"sync_sequence":            document.SyncSequence,
		"updated_at":               document.UpdatedAt,
	}
	_, err := repository.operations.UpdateOne(
		ctx,
		knowledgeBasesCollection,
		activeIDFilter(document.ID),
		bson.M{
			"$set": setValues,
			"$setOnInsert": bson.M{
				"_id":        document.ID,
				"deleted":    false,
				"created_at": document.CreatedAt,
			},
		},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

func (repository *KnowledgeBaseRepository) MarkDeleted(ctx context.Context, knowledgeBaseID string, deletedAt time.Time, reason string, syncSequence int64) error {
	result, err := repository.operations.UpdateOne(
		ctx,
		knowledgeBasesCollection,
		bson.M{"_id": knowledgeBaseID},
		bson.M{"$set": bson.M{
			"deleted":       true,
			"deleted_at":    deletedAt,
			"delete_reason": reason,
			"sync_sequence": syncSequence,
			"updated_at":    deletedAt,
		}},
	)
	return matchedOrNotFound(result, err)
}
