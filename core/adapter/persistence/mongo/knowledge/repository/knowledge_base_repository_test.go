package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	gomongo "go.mongodb.org/mongo-driver/v2/mongo"

	"myai/core/adapter/persistence/mongo/knowledge/po"
	mongotemplate "myai/core/adapter/persistence/mongo/template"
	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

func TestKnowledgeBaseGetFiltersLogicalDeletion(t *testing.T) {
	operations := &fakeOperations{base: po.KnowledgeBaseDocument{ID: "knowledge-1", Name: "MyAI"}}
	repository := NewKnowledgeBaseRepositoryWithOperations(operations)

	base, err := repository.Get(context.Background(), "knowledge-1")
	if err != nil {
		t.Fatal(err)
	}
	filter := operations.filter.(bson.M)
	if operations.collection != knowledgeBasesCollection || filter["_id"] != "knowledge-1" || filter["$or"] == nil {
		t.Fatalf("unexpected active get filter: %#v", filter)
	}
	if base.ID != "knowledge-1" {
		t.Fatalf("unexpected mapped base: %#v", base)
	}
}

func TestKnowledgeBaseGetTranslatesNotFound(t *testing.T) {
	repository := NewKnowledgeBaseRepositoryWithOperations(&fakeOperations{findOneErr: mongotemplate.ErrNotFound})
	_, err := repository.Get(context.Background(), "missing")
	if !errors.Is(err, knowledgeport.ErrNotFound) {
		t.Fatalf("Get() error = %v, want knowledge not found", err)
	}
}

func TestKnowledgeBaseSaveUsesActiveUpsert(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	operations := &fakeOperations{}
	repository := NewKnowledgeBaseRepositoryWithOperations(operations)
	base := domainknowledge.KnowledgeBase{
		ID:                   "knowledge-1",
		Name:                 "MyAI",
		RAGEnabled:           true,
		ActiveIndexProfileID: "index-1",
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	if err := repository.Save(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	filter := operations.filter.(bson.M)
	update := operations.update.(bson.M)
	if filter["$or"] == nil || len(operations.updateOptions) != 1 {
		t.Fatalf("save must upsert only an active record: filter=%#v options=%d", filter, len(operations.updateOptions))
	}
	insert := update["$setOnInsert"].(bson.M)
	if insert["deleted"] != false {
		t.Fatalf("new base must be active: %#v", insert)
	}
}

func TestKnowledgeBaseSaveRejectsDeletedDomainObject(t *testing.T) {
	now := time.Now()
	operations := &fakeOperations{}
	repository := NewKnowledgeBaseRepositoryWithOperations(operations)
	err := repository.Save(context.Background(), domainknowledge.KnowledgeBase{
		ID:                   "knowledge-1",
		Name:                 "MyAI",
		RAGEnabled:           true,
		ActiveIndexProfileID: "index-1",
		Deletion:             domainknowledge.Deletion{Deleted: true, DeletedAt: &now},
	})
	if err == nil || operations.updateCallCount != 0 {
		t.Fatalf("deleted base must not be saved: error=%v calls=%d", err, operations.updateCallCount)
	}
}

func TestKnowledgeBaseMarkDeletedRequiresExistingRecord(t *testing.T) {
	repository := NewKnowledgeBaseRepositoryWithOperations(&fakeOperations{
		updateResult: &gomongo.UpdateResult{MatchedCount: 0},
	})
	err := repository.MarkDeleted(context.Background(), "missing", time.Now(), "removed", 4)
	if !errors.Is(err, knowledgeport.ErrNotFound) {
		t.Fatalf("MarkDeleted() error = %v, want knowledge not found", err)
	}
}
