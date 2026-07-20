package sqlitevec

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	domainknowledge "myai/core/domain/knowledge"
)

func TestStoreUpsertSearchAndLogicalDelete(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	ctx := context.Background()
	profile := domainknowledge.VectorIndexDefinition{
		EmbeddingProfileID: "bge-m3-1024",
		Dimensions:         3,
		DistanceMetricID:   "cosine",
	}
	if err := store.EnsureIndex(ctx, profile); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := store.Upsert(ctx, []domainknowledge.EmbeddingVector{
		testEmbedding("embedding-a", "chunk-a", "kb-a", []float32{1, 0, 0}, 1),
		testEmbedding("embedding-b", "chunk-b", "kb-a", []float32{0, 1, 0}, 1),
		testEmbedding("embedding-c", "chunk-c", "kb-b", []float32{0, 0, 1}, 1),
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	hits, err := store.Search(ctx, domainknowledge.VectorQuery{
		EmbeddingProfileID: "bge-m3-1024",
		DistanceMetricID:   "cosine",
		Vector:             []float32{1, 0, 0},
		TopK:               2,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 2 || hits[0].ChunkID != "chunk-a" {
		t.Fatalf("unexpected hits: %#v", hits)
	}
	if err := store.MarkDeleted(ctx, domainknowledge.VectorDeletion{
		EmbeddingProfileID: "bge-m3-1024",
		EmbeddingIDs:       []string{"embedding-a"},
		SyncSequence:       2,
		DeletedAt:          time.Now().UTC(),
	}); err != nil {
		t.Fatalf("mark deleted: %v", err)
	}
	hits, err = store.Search(ctx, domainknowledge.VectorQuery{
		EmbeddingProfileID: "bge-m3-1024",
		DistanceMetricID:   "cosine",
		Vector:             []float32{1, 0, 0},
		TopK:               3,
	})
	if err != nil {
		t.Fatalf("search after delete: %v", err)
	}
	for _, hit := range hits {
		if hit.EmbeddingID == "embedding-a" {
			t.Fatalf("logically deleted embedding was returned: %#v", hits)
		}
	}
	if err := store.Upsert(ctx, []domainknowledge.EmbeddingVector{
		testEmbedding("embedding-a", "chunk-a", "kb-a", []float32{1, 0, 0}, 1),
	}); err != nil {
		t.Fatalf("stale upsert: %v", err)
	}
	hits, err = store.Search(ctx, domainknowledge.VectorQuery{
		EmbeddingProfileID: "bge-m3-1024",
		DistanceMetricID:   "cosine",
		Vector:             []float32{1, 0, 0},
		TopK:               3,
	})
	if err != nil {
		t.Fatalf("search after stale upsert: %v", err)
	}
	for _, hit := range hits {
		if hit.EmbeddingID == "embedding-a" {
			t.Fatalf("stale upsert resurrected deleted embedding: %#v", hits)
		}
	}
}

func TestStoreSeparatesKnowledgeBaseFilters(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	ctx := context.Background()
	if err := store.EnsureIndex(ctx, domainknowledge.VectorIndexDefinition{
		EmbeddingProfileID: "profile",
		Dimensions:         2,
		DistanceMetricID:   "l2",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(ctx, []domainknowledge.EmbeddingVector{
		testEmbeddingForProfile("profile", "a", "chunk-a", "kb-a", []float32{0, 0}, 1),
		testEmbeddingForProfile("profile", "b", "chunk-b", "kb-b", []float32{0.1, 0}, 1),
	}); err != nil {
		t.Fatal(err)
	}
	hits, err := store.Search(ctx, domainknowledge.VectorQuery{
		EmbeddingProfileID: "profile",
		DistanceMetricID:   "l2",
		KnowledgeBaseIDs:   []string{"kb-b"},
		Vector:             []float32{0, 0},
		TopK:               5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].EmbeddingID != "b" {
		t.Fatalf("unexpected filtered hits: %#v", hits)
	}
}

func TestStoreRejectsIncompatibleProfile(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	ctx := context.Background()
	definition := domainknowledge.VectorIndexDefinition{
		EmbeddingProfileID: "profile",
		Dimensions:         2,
		DistanceMetricID:   "cosine",
	}
	if err := store.EnsureIndex(ctx, definition); err != nil {
		t.Fatal(err)
	}
	definition.Dimensions = 3
	if err := store.EnsureIndex(ctx, definition); err == nil {
		t.Fatal("expected incompatible profile error")
	}
}

func TestStoreSupportsEscapedWindowsPathCharacters(t *testing.T) {
	store, err := Open(Config{Path: filepath.Join(t.TempDir(), "knowledge # cache", "vectors.db")})
	if err != nil {
		t.Fatalf("open sqlite-vec store with escaped path: %v", err)
	}
	defer store.Close()
	if err := store.Health(context.Background()); err != nil {
		t.Fatalf("health check: %v", err)
	}
}

func TestStoreSerializesConcurrentWriters(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	ctx := context.Background()
	if err := store.EnsureIndex(ctx, domainknowledge.VectorIndexDefinition{
		EmbeddingProfileID: "profile",
		Dimensions:         2,
		DistanceMetricID:   "cosine",
	}); err != nil {
		t.Fatal(err)
	}
	const writers = 16
	errorsChannel := make(chan error, writers)
	var waitGroup sync.WaitGroup
	for index := 0; index < writers; index++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			id := fmt.Sprintf("embedding-%02d", index)
			errorsChannel <- store.Upsert(ctx, []domainknowledge.EmbeddingVector{
				testEmbeddingForProfile("profile", id, "chunk-"+id, "kb", []float32{float32(index + 1), 1}, int64(index+1)),
			})
		}(index)
	}
	waitGroup.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatalf("concurrent upsert: %v", err)
		}
	}
	hits, err := store.Search(ctx, domainknowledge.VectorQuery{
		EmbeddingProfileID: "profile",
		DistanceMetricID:   "cosine",
		Vector:             []float32{1, 1},
		TopK:               writers,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != writers {
		t.Fatalf("expected %d hits, got %d", writers, len(hits))
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(Config{Path: filepath.Join(t.TempDir(), "vectors.db")})
	if err != nil {
		t.Fatalf("open sqlite-vec store: %v", err)
	}
	return store
}

func testEmbedding(id, chunkID, knowledgeBaseID string, values []float32, sequence int64) domainknowledge.EmbeddingVector {
	return testEmbeddingForProfile("bge-m3-1024", id, chunkID, knowledgeBaseID, values, sequence)
}

func testEmbeddingForProfile(profileID, id, chunkID, knowledgeBaseID string, values []float32, sequence int64) domainknowledge.EmbeddingVector {
	return domainknowledge.EmbeddingVector{
		ID:                 id,
		ChunkID:            chunkID,
		KnowledgeBaseID:    knowledgeBaseID,
		DocumentID:         "document-1",
		DocumentVersion:    1,
		EmbeddingProfileID: profileID,
		Dimensions:         len(values),
		Values:             values,
		SyncSequence:       sequence,
	}
}
