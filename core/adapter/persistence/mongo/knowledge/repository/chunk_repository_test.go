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

func TestChunkSaveAllValidatesBeforeCallingMongo(t *testing.T) {
	operations := &fakeOperations{}
	repository := NewChunkRepositoryWithOperations(operations)

	err := repository.SaveAll(context.Background(), []domainknowledge.Chunk{{ID: "invalid"}})
	if err == nil || operations.updateCallCount != 0 {
		t.Fatalf("invalid chunk must not reach Mongo: err=%v calls=%d", err, operations.updateCallCount)
	}
}

func TestChunkSaveAllPreservesEmbeddingStateOnUpsert(t *testing.T) {
	operations := &fakeOperations{}
	repository := NewChunkRepositoryWithOperations(operations)
	now := time.Now().UTC()
	chunk := validChunk()
	chunk.CreatedAt = now
	chunk.UpdatedAt = now
	chunk.EmbeddingProfileIDs = []string{"embedding-a"}

	if err := repository.SaveAll(context.Background(), []domainknowledge.Chunk{chunk}); err != nil {
		t.Fatal(err)
	}
	update, ok := operations.update.(bson.M)
	if !ok {
		t.Fatalf("expected BSON update document, got %#v", operations.update)
	}
	setValues, ok := update["$set"].(bson.M)
	if !ok || setValues["text"] != chunk.Text {
		t.Fatalf("unexpected chunk set values: %#v", update)
	}
	setOnInsert, ok := update["$setOnInsert"].(bson.M)
	if !ok || setOnInsert["embedding_profile_ids"] == nil {
		t.Fatalf("expected embedding state only on insert: %#v", update)
	}
}

func TestChunkListMissingEmbeddingsMapsDocuments(t *testing.T) {
	operations := &fakeOperations{chunks: []po.ChunkDocument{{
		ID:                  "chunk-1",
		KnowledgeBaseID:     "knowledge-1",
		DocumentID:          "document-1",
		DocumentVersion:     1,
		ParsingProfileID:    "parsing-1",
		ChunkingProfileID:   "chunking-1",
		Ordinal:             0,
		Text:                "text",
		ContentHash:         "hash",
		EndOffset:           4,
		EmbeddingProfileIDs: []string{"embedding-old"},
	}}}
	repository := NewChunkRepositoryWithOperations(operations)

	chunks, err := repository.ListMissingEmbeddings(context.Background(), "parsing-1", "chunking-1", "embedding-new", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 || chunks[0].ID != "chunk-1" || chunks[0].EmbeddingProfileIDs[0] != "embedding-old" {
		t.Fatalf("unexpected missing embedding chunks: %#v", chunks)
	}
	if filter, ok := operations.filter.(bson.M); !ok || filter["embedding_profile_ids"] == nil {
		t.Fatalf("expected missing profile filter, got %#v", operations.filter)
	}
}

func TestChunkListByDocumentPageUsesOrdinalCursor(t *testing.T) {
	operations := &fakeOperations{chunks: []po.ChunkDocument{{
		ID:                "chunk-2",
		KnowledgeBaseID:   "knowledge-1",
		DocumentID:        "document-1",
		DocumentVersion:   1,
		ParsingProfileID:  "parsing-1",
		ChunkingProfileID: "chunking-1",
		Ordinal:           2,
		Text:              "text",
		ContentHash:       "hash",
		EndOffset:         4,
	}}}
	repository := NewChunkRepositoryWithOperations(operations)

	chunks, err := repository.ListByDocumentPage(context.Background(), "document-1", 1, "parsing-1", "chunking-1", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 || chunks[0].Ordinal != 2 {
		t.Fatalf("unexpected chunk page: %#v", chunks)
	}
	filter, ok := operations.filter.(bson.M)
	if !ok {
		t.Fatalf("expected BSON filter, got %#v", operations.filter)
	}
	ordinal, ok := filter["ordinal"].(bson.M)
	if !ok || ordinal["$gt"] != 1 {
		t.Fatalf("expected ordinal cursor filter, got %#v", filter)
	}
}

func TestChunkMarkEmbeddedUsesAddToSet(t *testing.T) {
	operations := &fakeOperations{}
	repository := NewChunkRepositoryWithOperations(operations)
	updatedAt := time.Now().UTC()

	if err := repository.MarkEmbedded(context.Background(), []string{"chunk-1", "chunk-2"}, "embedding-new", updatedAt); err != nil {
		t.Fatal(err)
	}
	update, ok := operations.updateMany.(bson.M)
	if !ok {
		t.Fatalf("expected update many document, got %#v", operations.updateMany)
	}
	addToSet, ok := update["$addToSet"].(bson.M)
	if !ok || addToSet["embedding_profile_ids"] != "embedding-new" {
		t.Fatalf("expected addToSet for embedding profile, got %#v", update)
	}
}

func TestChunkMarkDeletedByDocumentUsesLogicalDeletion(t *testing.T) {
	operations := &fakeOperations{}
	repository := NewChunkRepositoryWithOperations(operations)
	deletedAt := time.Now().UTC()

	if err := repository.MarkDeletedByDocument(context.Background(), "document-1", deletedAt, 7); err != nil {
		t.Fatal(err)
	}
	update, ok := operations.updateMany.(bson.M)
	if !ok {
		t.Fatalf("expected update many document, got %#v", operations.updateMany)
	}
	setValues, ok := update["$set"].(bson.M)
	if !ok || setValues["deleted"] != true || setValues["sync_sequence"] != int64(7) {
		t.Fatalf("unexpected deletion update: %#v", update)
	}
}

func TestChunkRepositoryRejectsInvalidQueries(t *testing.T) {
	repository := NewChunkRepositoryWithOperations(&fakeOperations{})
	if _, err := repository.GetByIDs(context.Background(), []string{"", "chunk-1"}); err == nil {
		t.Fatal("expected empty chunk id error")
	}
	if _, err := repository.ListMissingEmbeddings(context.Background(), "parsing-1", "chunking-1", "embedding-1", 0); err == nil {
		t.Fatal("expected non-positive limit error")
	}
	if _, err := repository.ListByDocumentPage(context.Background(), "document-1", 1, "parsing-1", "chunking-1", -2, 10); err == nil {
		t.Fatal("expected invalid ordinal cursor error")
	}
	if err := repository.MarkEmbedded(context.Background(), nil, "embedding-1", time.Now()); err != nil {
		t.Fatalf("empty chunk list should be a no-op: %v", err)
	}
	if strings.Contains(strings.ToLower(repositoryError(gomongo.ErrNoDocuments).Error()), "mongo") {
		t.Fatal("repository error should expose domain error")
	}
}

func validChunk() domainknowledge.Chunk {
	return domainknowledge.Chunk{
		ID:                "chunk-1",
		KnowledgeBaseID:   "knowledge-1",
		DocumentID:        "document-1",
		DocumentVersion:   1,
		ParsingProfileID:  "parsing-1",
		ChunkingProfileID: "chunking-1",
		Ordinal:           0,
		Text:              "chunk text",
		ContentHash:       "chunk-hash",
		EndOffset:         10,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
}
