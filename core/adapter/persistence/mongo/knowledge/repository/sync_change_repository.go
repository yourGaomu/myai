package repository

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	knowledgemapper "myai/core/adapter/persistence/mongo/knowledge/mapper"
	"myai/core/adapter/persistence/mongo/knowledge/po"
	mongotemplate "myai/core/adapter/persistence/mongo/template"
	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

type SyncChangeRepository struct {
	operations mongotemplate.Operations
}

var _ knowledgeport.SyncChangeRepository = (*SyncChangeRepository)(nil)

func NewSyncChangeRepository(client *gomongo.Client, database string) *SyncChangeRepository {
	if client == nil || database == "" {
		return NewSyncChangeRepositoryWithOperations(mongotemplate.New(nil))
	}
	return NewSyncChangeRepositoryWithOperations(mongotemplate.New(client.Database(database)))
}

func NewSyncChangeRepositoryWithOperations(operations mongotemplate.Operations) *SyncChangeRepository {
	if operations == nil {
		operations = mongotemplate.New(nil)
	}
	return &SyncChangeRepository{operations: operations}
}

func (repository *SyncChangeRepository) Append(ctx context.Context, change domainknowledge.SyncChange) error {
	if err := change.Validate(); err != nil {
		return fmt.Errorf("validate knowledge sync change: %w", err)
	}
	document := knowledgemapper.SyncChangeDocumentFromDomain(change)
	if _, err := repository.operations.InsertOne(ctx, syncChangesCollection, document); err != nil {
		return fmt.Errorf("append knowledge sync change %q: %w", document.ID, repositoryError(err))
	}
	return nil
}

func (repository *SyncChangeRepository) ListAfter(ctx context.Context, sequence int64, limit int) ([]domainknowledge.SyncChange, error) {
	if sequence < 0 {
		return nil, fmt.Errorf("sequence must not be negative")
	}
	if limit < 1 {
		return nil, fmt.Errorf("limit must be positive")
	}
	var documents []po.SyncChangeDocument
	if err := repository.operations.FindAll(
		ctx,
		syncChangesCollection,
		bson.M{"sequence": bson.M{"$gt": sequence}},
		&documents,
		options.Find().SetSort(bson.D{{Key: "sequence", Value: 1}}).SetLimit(int64(limit)),
	); err != nil {
		return nil, repositoryError(err)
	}
	result := make([]domainknowledge.SyncChange, 0, len(documents))
	for _, document := range documents {
		result = append(result, knowledgemapper.SyncChangeDomainFromDocument(document))
	}
	return result, nil
}
