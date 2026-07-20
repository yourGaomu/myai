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

type IndexingJobRepository struct {
	operations mongotemplate.Operations
}

var _ knowledgeport.IndexingJobRepository = (*IndexingJobRepository)(nil)

func NewIndexingJobRepository(client *gomongo.Client, database string) *IndexingJobRepository {
	if client == nil || database == "" {
		return NewIndexingJobRepositoryWithOperations(mongotemplate.New(nil))
	}
	return NewIndexingJobRepositoryWithOperations(mongotemplate.New(client.Database(database)))
}

func NewIndexingJobRepositoryWithOperations(operations mongotemplate.Operations) *IndexingJobRepository {
	if operations == nil {
		operations = mongotemplate.New(nil)
	}
	return &IndexingJobRepository{operations: operations}
}

func (repository *IndexingJobRepository) Get(ctx context.Context, jobID string) (domainknowledge.IndexingJob, error) {
	var document po.IndexingJobDocument
	if err := repository.operations.FindOne(ctx, indexingJobsCollection, bson.M{"_id": jobID}, &document); err != nil {
		return domainknowledge.IndexingJob{}, repositoryError(err)
	}
	return knowledgemapper.IndexingJobDomainFromDocument(document), nil
}

func (repository *IndexingJobRepository) Save(ctx context.Context, job domainknowledge.IndexingJob) error {
	if err := job.Validate(); err != nil {
		return fmt.Errorf("validate indexing job: %w", err)
	}
	document := knowledgemapper.IndexingJobDocumentFromDomain(job)
	_, err := repository.operations.UpdateOne(
		ctx,
		indexingJobsCollection,
		bson.M{"_id": document.ID},
		bson.M{
			"$set": bson.M{
				"knowledge_base_id": document.KnowledgeBaseID,
				"document_id":       document.DocumentID,
				"index_profile_id":  document.IndexProfileID,
				"stage":             document.Stage,
				"status":            document.Status,
				"total_chunks":      document.TotalChunks,
				"completed_chunks":  document.CompletedChunks,
				"failed_chunks":     document.FailedChunks,
				"last_error":        document.LastError,
				"retry_count":       document.RetryCount,
				"updated_at":        document.UpdatedAt,
				"completed_at":      document.CompletedAt,
			},
			"$setOnInsert": bson.M{
				"_id":        document.ID,
				"created_at": document.CreatedAt,
			},
		},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("save indexing job %q: %w", document.ID, repositoryError(err))
	}
	return nil
}

func (repository *IndexingJobRepository) ListPending(ctx context.Context, limit int) ([]domainknowledge.IndexingJob, error) {
	if limit < 1 {
		return nil, fmt.Errorf("limit must be positive")
	}
	filter := bson.M{"status": bson.M{"$in": bson.A{
		string(domainknowledge.IndexingJobStatusPending),
		string(domainknowledge.IndexingJobStatusRunning),
	}}}
	var documents []po.IndexingJobDocument
	if err := repository.operations.FindAll(
		ctx,
		indexingJobsCollection,
		filter,
		&documents,
		options.Find().SetSort(bson.D{{Key: "updated_at", Value: 1}}).SetLimit(int64(limit)),
	); err != nil {
		return nil, repositoryError(err)
	}
	result := make([]domainknowledge.IndexingJob, 0, len(documents))
	for _, document := range documents {
		result = append(result, knowledgemapper.IndexingJobDomainFromDocument(document))
	}
	return result, nil
}

func (repository *IndexingJobRepository) ListByKnowledgeBase(ctx context.Context, knowledgeBaseID string, limit int) ([]domainknowledge.IndexingJob, error) {
	if limit < 1 {
		return nil, fmt.Errorf("limit must be positive")
	}
	filter := bson.M{"knowledge_base_id": knowledgeBaseID}
	var documents []po.IndexingJobDocument
	if err := repository.operations.FindAll(ctx, indexingJobsCollection, filter, &documents, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}).SetLimit(int64(limit))); err != nil {
		return nil, repositoryError(err)
	}
	result := make([]domainknowledge.IndexingJob, 0, len(documents))
	for _, document := range documents {
		result = append(result, knowledgemapper.IndexingJobDomainFromDocument(document))
	}
	return result, nil
}
