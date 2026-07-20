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

type DocumentRepository struct {
	operations mongotemplate.Operations
}

var _ knowledgeport.DocumentRepository = (*DocumentRepository)(nil)

func NewDocumentRepository(client *gomongo.Client, database string) *DocumentRepository {
	if client == nil || database == "" {
		return NewDocumentRepositoryWithOperations(mongotemplate.New(nil))
	}
	return NewDocumentRepositoryWithOperations(mongotemplate.New(client.Database(database)))
}

func NewDocumentRepositoryWithOperations(operations mongotemplate.Operations) *DocumentRepository {
	if operations == nil {
		operations = mongotemplate.New(nil)
	}
	return &DocumentRepository{operations: operations}
}

func (repository *DocumentRepository) Get(ctx context.Context, documentID string) (domainknowledge.Document, error) {
	var document po.KnowledgeDocument
	if err := repository.operations.FindOne(ctx, documentsCollection, activeIDFilter(documentID), &document); err != nil {
		return domainknowledge.Document{}, repositoryError(err)
	}
	return knowledgemapper.KnowledgeDocumentDomainFromDocument(document), nil
}

func (repository *DocumentRepository) ListByKnowledgeBase(ctx context.Context, knowledgeBaseID string, includeDeleted bool) ([]domainknowledge.Document, error) {
	filter := activeFilter()
	filter["knowledge_base_id"] = knowledgeBaseID
	if includeDeleted {
		filter = bson.M{"knowledge_base_id": knowledgeBaseID}
	}

	var documents []po.KnowledgeDocument
	if err := repository.operations.FindAll(
		ctx,
		documentsCollection,
		filter,
		&documents,
		options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}),
	); err != nil {
		return nil, err
	}
	result := make([]domainknowledge.Document, 0, len(documents))
	for _, document := range documents {
		result = append(result, knowledgemapper.KnowledgeDocumentDomainFromDocument(document))
	}
	return result, nil
}

func (repository *DocumentRepository) Save(ctx context.Context, document domainknowledge.Document) error {
	if err := document.Validate(); err != nil {
		return fmt.Errorf("validate knowledge document: %w", err)
	}
	if document.Deletion.Deleted {
		return fmt.Errorf("save cannot persist a deleted knowledge document; use MarkDeleted")
	}

	persistent := knowledgemapper.KnowledgeDocumentFromDomain(document)
	_, err := repository.operations.UpdateOne(
		ctx,
		documentsCollection,
		activeIDFilter(persistent.ID),
		bson.M{
			"$set": bson.M{
				"knowledge_base_id": persistent.KnowledgeBaseID,
				"file_name":         persistent.FileName,
				"content_type":      persistent.ContentType,
				"object_key":        persistent.ObjectKey,
				"content_hash":      persistent.ContentHash,
				"version":           persistent.Version,
				"status":            persistent.Status,
				"failure_reason":    persistent.FailureReason,
				"sync_sequence":     persistent.SyncSequence,
				"updated_at":        persistent.UpdatedAt,
			},
			"$setOnInsert": bson.M{
				"_id":        persistent.ID,
				"deleted":    false,
				"created_at": persistent.CreatedAt,
			},
		},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

func (repository *DocumentRepository) MarkDeleted(ctx context.Context, documentID string, deletedAt time.Time, reason string, syncSequence int64) error {
	result, err := repository.operations.UpdateOne(
		ctx,
		documentsCollection,
		bson.M{"_id": documentID},
		bson.M{"$set": bson.M{
			"status":        string(domainknowledge.DocumentStatusDeleted),
			"deleted":       true,
			"deleted_at":    deletedAt,
			"delete_reason": reason,
			"sync_sequence": syncSequence,
			"updated_at":    deletedAt,
		}},
	)
	return matchedOrNotFound(result, err)
}
