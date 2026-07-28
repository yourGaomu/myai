package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	domainknowledge "myai/core/domain/knowledge"
)

func TestIndexingStateRepositoryValidatesPairBeforeTransaction(t *testing.T) {
	document := domainknowledge.Document{
		ID:              "document-1",
		KnowledgeBaseID: "knowledge-1",
		FileName:        "guide.md",
		ContentType:     "text/markdown",
		ObjectKey:       "knowledge/knowledge-1/document-1/v1/guide.md",
		ContentHash:     "content-hash",
		Version:         1,
		Status:          domainknowledge.DocumentStatusUploaded,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	job := validIndexingJob()
	job.DocumentID = "another-document"

	err := (&IndexingStateRepository{}).SaveDocumentAndJob(context.Background(), document, job)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected mismatched state error, got %v", err)
	}
}
