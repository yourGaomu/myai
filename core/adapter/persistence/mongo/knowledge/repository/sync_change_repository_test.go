package repository

import (
	"context"
	"testing"
	"time"

	"myai/core/adapter/persistence/mongo/knowledge/po"
	domainknowledge "myai/core/domain/knowledge"
)

func TestSyncChangeRepositoryAppendsAndListsAfterSequence(t *testing.T) {
	operations := &fakeOperations{syncChanges: []po.SyncChangeDocument{{
		ID:              "change-2",
		Sequence:        2,
		KnowledgeBaseID: "knowledge-1",
		EntityType:      "chunk",
		EntityID:        "chunk-2",
		Operation:       string(domainknowledge.SyncOperationUpsert),
		EntityVersion:   1,
		OccurredAt:      time.Now().UTC(),
	}}}
	repository := NewSyncChangeRepositoryWithOperations(operations)
	change := domainknowledge.SyncChange{
		ID:              "change-1",
		Sequence:        1,
		KnowledgeBaseID: "knowledge-1",
		EntityType:      "chunk",
		EntityID:        "chunk-1",
		Operation:       domainknowledge.SyncOperationUpsert,
		EntityVersion:   1,
		OccurredAt:      time.Now().UTC(),
	}
	if err := repository.Append(context.Background(), change); err != nil {
		t.Fatal(err)
	}
	if operations.insertCollection != syncChangesCollection {
		t.Fatalf("unexpected sync change collection: %q", operations.insertCollection)
	}
	if _, ok := operations.insertDocument.(po.SyncChangeDocument); !ok {
		t.Fatalf("expected sync change PO, got %#v", operations.insertDocument)
	}

	changes, err := repository.ListAfter(context.Background(), 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Sequence != 2 {
		t.Fatalf("unexpected changes after sequence: %#v", changes)
	}
}

func TestSyncChangeRepositoryValidatesAppendAndQuery(t *testing.T) {
	operations := &fakeOperations{}
	repository := NewSyncChangeRepositoryWithOperations(operations)
	if err := repository.Append(context.Background(), domainknowledge.SyncChange{ID: "invalid"}); err == nil {
		t.Fatal("expected invalid sync change error")
	}
	if _, err := repository.ListAfter(context.Background(), -1, 10); err == nil {
		t.Fatal("expected negative sequence error")
	}
	if _, err := repository.ListAfter(context.Background(), 0, 0); err == nil {
		t.Fatal("expected non-positive limit error")
	}
	if operations.insertDocument != nil {
		t.Fatal("invalid sync change must not reach Mongo")
	}
}
