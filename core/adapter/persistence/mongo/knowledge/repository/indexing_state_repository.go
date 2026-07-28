package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gomongo "go.mongodb.org/mongo-driver/v2/mongo"

	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

// IndexingStateRepository commits document/job transitions in one Mongo
// transaction. The Mongo deployment must support transactions.
type IndexingStateRepository struct {
	database  *gomongo.Database
	documents *DocumentRepository
	jobs      *IndexingJobRepository
}

var _ knowledgeport.IndexingStateRepository = (*IndexingStateRepository)(nil)

func NewIndexingStateRepository(client *gomongo.Client, database string) *IndexingStateRepository {
	if client == nil || strings.TrimSpace(database) == "" {
		return &IndexingStateRepository{}
	}
	target := client.Database(database)
	return &IndexingStateRepository{
		database:  target,
		documents: NewDocumentRepository(client, database),
		jobs:      NewIndexingJobRepository(client, database),
	}
}

func (repository *IndexingStateRepository) SaveDocumentAndJob(ctx context.Context, document domainknowledge.Document, job domainknowledge.IndexingJob) error {
	if err := validateDocumentJobState(document, job); err != nil {
		return err
	}
	if repository == nil || repository.database == nil || repository.documents == nil || repository.jobs == nil {
		return errors.New("mongo knowledge indexing state repository is not configured")
	}
	session, err := repository.database.Client().StartSession()
	if err != nil {
		return fmt.Errorf("start knowledge indexing transaction: %w", err)
	}
	defer session.EndSession(context.Background())

	_, err = session.WithTransaction(ctx, func(transactionContext context.Context) (any, error) {
		if err := repository.documents.Save(transactionContext, document); err != nil {
			return nil, err
		}
		if err := repository.jobs.Save(transactionContext, job); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return fmt.Errorf("commit knowledge indexing state: %w", err)
	}
	return nil
}

func validateDocumentJobState(document domainknowledge.Document, job domainknowledge.IndexingJob) error {
	if err := document.Validate(); err != nil {
		return fmt.Errorf("validate indexing state document: %w", err)
	}
	if err := job.Validate(); err != nil {
		return fmt.Errorf("validate indexing state job: %w", err)
	}
	if document.ID != job.DocumentID || document.KnowledgeBaseID != job.KnowledgeBaseID {
		return fmt.Errorf("indexing job %q does not match document %q", job.ID, document.ID)
	}
	return nil
}
