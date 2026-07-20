package mapper

import (
	"reflect"
	"testing"
	"time"

	domainknowledge "myai/core/domain/knowledge"
)

func TestIndexingJobAndSyncChangeRoundTrip(t *testing.T) {
	now := time.Now().UTC()
	completedAt := now.Add(time.Minute)
	job := domainknowledge.IndexingJob{
		ID:              "job-1",
		KnowledgeBaseID: "knowledge-1",
		DocumentID:      "document-1",
		IndexProfileID:  "index-1",
		Stage:           domainknowledge.IndexingStageEmbed,
		Status:          domainknowledge.IndexingJobStatusCompleted,
		TotalChunks:     2,
		CompletedChunks: 2,
		CompletedAt:     &completedAt,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if mapped := IndexingJobDomainFromDocument(IndexingJobDocumentFromDomain(job)); !reflect.DeepEqual(mapped, job) {
		t.Fatalf("indexing job round trip changed value: %#v != %#v", mapped, job)
	}

	change := domainknowledge.SyncChange{
		ID:              "change-1",
		Sequence:        3,
		KnowledgeBaseID: "knowledge-1",
		EntityType:      "chunk",
		EntityID:        "chunk-1",
		Operation:       domainknowledge.SyncOperationDelete,
		EntityVersion:   1,
		Deleted:         true,
		OccurredAt:      now,
	}
	if mapped := SyncChangeDomainFromDocument(SyncChangeDocumentFromDomain(change)); !reflect.DeepEqual(mapped, change) {
		t.Fatalf("sync change round trip changed value: %#v != %#v", mapped, change)
	}
}
