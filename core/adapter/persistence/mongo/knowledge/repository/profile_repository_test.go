package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	gomongo "go.mongodb.org/mongo-driver/v2/mongo"

	"myai/core/adapter/persistence/mongo/knowledge/po"
	domainknowledge "myai/core/domain/knowledge"
)

func TestProfileRepositorySavesAndSoftDeletesEmbeddingProfile(t *testing.T) {
	operations := &fakeOperations{}
	repository := NewProfileRepositoryWithOperations(operations)
	profile := domainknowledge.EmbeddingProfile{
		ID:         "embedding-1",
		Name:       "Embedding",
		ModelID:    "model-1",
		Provider:   "provider",
		Model:      "embedding-model",
		Dimensions: 1024,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	if err := repository.SaveEmbeddingProfile(context.Background(), profile); err != nil {
		t.Fatal(err)
	}
	if operations.collection != embeddingProfilesCollection || operations.updateCallCount != 1 {
		t.Fatalf("unexpected save call: %#v", operations)
	}

	if err := repository.MarkEmbeddingProfileDeleted(context.Background(), profile.ID, time.Now().UTC(), "retired"); err != nil {
		t.Fatal(err)
	}
	update, ok := operations.update.(bson.M)
	if !ok {
		t.Fatalf("expected deletion update, got %#v", operations.update)
	}
	setValues, ok := update["$set"].(bson.M)
	if !ok || setValues["deleted"] != true || setValues["delete_reason"] != "retired" {
		t.Fatalf("unexpected profile deletion update: %#v", update)
	}
}

func TestProfileRepositorySavesAndLoadsParsingProfile(t *testing.T) {
	operations := &fakeOperations{parsingProfile: po.ParsingProfileDocument{
		ID:            "parsing-1",
		Name:          "Python",
		ParserID:      "python",
		ParserVersion: "1",
	}}
	repository := NewProfileRepositoryWithOperations(operations)
	profile := domainknowledge.ParsingProfile{
		ID:            "parsing-1",
		Name:          "Python",
		ParserID:      "python",
		ParserVersion: "1",
		Options:       map[string]string{"language": "zh"},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := repository.SaveParsingProfile(context.Background(), profile); err != nil {
		t.Fatal(err)
	}
	if operations.collection != parsingProfilesCollection {
		t.Fatalf("unexpected parsing profile collection: %q", operations.collection)
	}

	loaded, err := repository.GetParsingProfile(context.Background(), profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ParserID != "python" || loaded.ParserVersion != "1" {
		t.Fatalf("unexpected parsing profile: %#v", loaded)
	}
}

func TestProfileRepositoryRejectsInvalidAndDeletedProfiles(t *testing.T) {
	operations := &fakeOperations{}
	repository := NewProfileRepositoryWithOperations(operations)
	if err := repository.SaveIndexProfile(context.Background(), domainknowledge.IndexProfile{ID: "index-1"}); err == nil {
		t.Fatal("expected invalid index profile error")
	}
	deletedAt := time.Now().UTC()
	profile := domainknowledge.EmbeddingProfile{
		ID:         "embedding-1",
		Name:       "Embedding",
		ModelID:    "model-1",
		Provider:   "provider",
		Model:      "embedding-model",
		Dimensions: 1024,
		Deletion:   domainknowledge.Deletion{Deleted: true, DeletedAt: &deletedAt},
	}
	if err := repository.SaveEmbeddingProfile(context.Background(), profile); err == nil {
		t.Fatal("deleted profile must not be persisted")
	}
	if operations.updateCallCount != 0 {
		t.Fatalf("invalid/deleted profile must not reach Mongo: calls=%d", operations.updateCallCount)
	}
}

func TestProfileRepositoryTranslatesNotFound(t *testing.T) {
	repository := NewProfileRepositoryWithOperations(&fakeOperations{findOneErr: gomongo.ErrNoDocuments})
	_, err := repository.GetIndexProfile(context.Background(), "index-1")
	if !strings.Contains(err.Error(), "knowledge resource not found") {
		t.Fatalf("expected domain not found error, got %v", err)
	}
}
