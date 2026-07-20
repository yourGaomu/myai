package repository

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"myai/core/adapter/persistence/mongo/knowledge/po"
	domainknowledge "myai/core/domain/knowledge"
)

func TestIndexingJobRepositorySavesAndLoadsRecoverableJobs(t *testing.T) {
	operations := &fakeOperations{indexingJobs: []po.IndexingJobDocument{{
		ID:              "job-1",
		KnowledgeBaseID: "knowledge-1",
		DocumentID:      "document-1",
		IndexProfileID:  "index-1",
		Stage:           string(domainknowledge.IndexingStageEmbed),
		Status:          string(domainknowledge.IndexingJobStatusRunning),
		TotalChunks:     2,
		CompletedChunks: 1,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}}}
	repository := NewIndexingJobRepositoryWithOperations(operations)
	job := validIndexingJob()
	if err := repository.Save(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if operations.collection != indexingJobsCollection || operations.updateCallCount != 1 {
		t.Fatalf("unexpected job save call: %#v", operations)
	}
	if update, ok := operations.update.(bson.M); !ok || update["$set"] == nil || update["$setOnInsert"] == nil {
		t.Fatalf("expected upsert update, got %#v", operations.update)
	}

	jobs, err := repository.ListPending(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Status != domainknowledge.IndexingJobStatusRunning {
		t.Fatalf("expected running job to be recoverable, got %#v", jobs)
	}
}

func TestIndexingJobRepositoryValidatesLimitAndJob(t *testing.T) {
	operations := &fakeOperations{}
	repository := NewIndexingJobRepositoryWithOperations(operations)
	if err := repository.Save(context.Background(), domainknowledge.IndexingJob{ID: "job-1"}); err == nil {
		t.Fatal("expected invalid indexing job error")
	}
	if _, err := repository.ListPending(context.Background(), 0); err == nil {
		t.Fatal("expected non-positive limit error")
	}
	if operations.updateCallCount != 0 {
		t.Fatalf("invalid job must not reach Mongo: calls=%d", operations.updateCallCount)
	}
}

func validIndexingJob() domainknowledge.IndexingJob {
	now := time.Now().UTC()
	return domainknowledge.IndexingJob{
		ID:              "job-1",
		KnowledgeBaseID: "knowledge-1",
		DocumentID:      "document-1",
		IndexProfileID:  "index-1",
		Stage:           domainknowledge.IndexingStageEmbed,
		Status:          domainknowledge.IndexingJobStatusRunning,
		TotalChunks:     2,
		CompletedChunks: 1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}
