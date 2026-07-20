package repository

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"myai/core/adapter/persistence/mongo/knowledge/po"
	domainknowledge "myai/core/domain/knowledge"
)

func TestDocumentListScopesKnowledgeBaseAndLogicalDeletion(t *testing.T) {
	operations := &fakeOperations{documents: []po.KnowledgeDocument{{ID: "document-1"}}}
	repository := NewDocumentRepositoryWithOperations(operations)

	documents, err := repository.ListByKnowledgeBase(context.Background(), "knowledge-1", false)
	if err != nil {
		t.Fatal(err)
	}
	filter := operations.filter.(bson.M)
	if filter["knowledge_base_id"] != "knowledge-1" || filter["$or"] == nil {
		t.Fatalf("unexpected list filter: %#v", filter)
	}
	if len(documents) != 1 || documents[0].ID != "document-1" {
		t.Fatalf("unexpected documents: %#v", documents)
	}
}

func TestDocumentSaveValidatesBeforeMongo(t *testing.T) {
	operations := &fakeOperations{}
	repository := NewDocumentRepositoryWithOperations(operations)
	if err := repository.Save(context.Background(), domainknowledge.Document{}); err == nil {
		t.Fatal("expected invalid document to fail")
	}
	if operations.updateCallCount != 0 {
		t.Fatalf("invalid document must not reach Mongo, calls=%d", operations.updateCallCount)
	}
}

func TestDocumentMarkDeletedSetsStatusAndTombstone(t *testing.T) {
	operations := &fakeOperations{}
	repository := NewDocumentRepositoryWithOperations(operations)
	deletedAt := time.Date(2026, 7, 19, 13, 0, 0, 0, time.UTC)

	if err := repository.MarkDeleted(context.Background(), "document-1", deletedAt, "removed", 7); err != nil {
		t.Fatal(err)
	}
	update := operations.update.(bson.M)
	set := update["$set"].(bson.M)
	if set["status"] != string(domainknowledge.DocumentStatusDeleted) || set["deleted"] != true || set["sync_sequence"] != int64(7) {
		t.Fatalf("unexpected delete update: %#v", set)
	}
}
