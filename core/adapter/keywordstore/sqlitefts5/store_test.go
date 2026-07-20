package sqlitefts5

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	domainknowledge "myai/core/domain/knowledge"
)

func TestStoreIndexesSearchesAndFiltersKnowledgeBases(t *testing.T) {
	store := openTestStore(t)
	documents := []domainknowledge.KeywordDocument{
		keywordDocument("chunk-1", "knowledge-a", "Redis vector search", 1),
		keywordDocument("chunk-2", "knowledge-b", "Redis session cache", 1),
		keywordDocument("chunk-3", "knowledge-a", "Mongo document storage", 1),
	}
	if err := store.Upsert(context.Background(), documents); err != nil {
		t.Fatal(err)
	}
	hits, err := store.Search(context.Background(), domainknowledge.KeywordQuery{
		Text: "Redis", KnowledgeBaseIDs: []string{"knowledge-a"}, TopK: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ChunkID != "chunk-1" || hits[0].Rank != 1 || hits[0].Score < 0 {
		t.Fatalf("unexpected FTS5 hits: %#v", hits)
	}
}

func TestStoreUpdateTriggerReplacesIndexedText(t *testing.T) {
	store := openTestStore(t)
	if err := store.Upsert(context.Background(), []domainknowledge.KeywordDocument{
		keywordDocument("chunk-1", "knowledge-a", "legacy redis text", 1),
	}); err != nil {
		t.Fatal(err)
	}
	updated := keywordDocument("chunk-1", "knowledge-a", "modern vector text", 2)
	if err := store.Upsert(context.Background(), []domainknowledge.KeywordDocument{updated}); err != nil {
		t.Fatal(err)
	}
	oldHits, err := store.Search(context.Background(), domainknowledge.KeywordQuery{Text: "legacy", TopK: 10})
	if err != nil {
		t.Fatal(err)
	}
	newHits, err := store.Search(context.Background(), domainknowledge.KeywordQuery{Text: "modern", TopK: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(oldHits) != 0 || len(newHits) != 1 || newHits[0].ChunkID != "chunk-1" {
		t.Fatalf("FTS update trigger is inconsistent: old=%#v new=%#v", oldHits, newHits)
	}
}

func TestStoreRejectsStaleUpsertAndDeletion(t *testing.T) {
	store := openTestStore(t)
	current := keywordDocument("chunk-1", "knowledge-a", "current content", 10)
	if err := store.Upsert(context.Background(), []domainknowledge.KeywordDocument{current}); err != nil {
		t.Fatal(err)
	}
	stale := keywordDocument("chunk-1", "knowledge-a", "stale content", 9)
	if err := store.Upsert(context.Background(), []domainknowledge.KeywordDocument{stale}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDeleted(context.Background(), []string{"chunk-1"}, 9); err != nil {
		t.Fatal(err)
	}
	hits, err := store.Search(context.Background(), domainknowledge.KeywordQuery{Text: "current", TopK: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("stale changes must not replace current document: %#v", hits)
	}
	staleHits, err := store.Search(context.Background(), domainknowledge.KeywordQuery{Text: "stale", TopK: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(staleHits) != 0 {
		t.Fatalf("stale text reached FTS index: %#v", staleHits)
	}
	if err := store.MarkDeleted(context.Background(), []string{"chunk-1"}, 10); err != nil {
		t.Fatal(err)
	}
	hits, err = store.Search(context.Background(), domainknowledge.KeywordQuery{Text: "current", TopK: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("logically deleted document must be filtered: %#v", hits)
	}
}

func TestStoreHandlesQuotedQueryAndChineseText(t *testing.T) {
	store := openTestStore(t)
	if err := store.Upsert(context.Background(), []domainknowledge.KeywordDocument{
		keywordDocument("chunk-1", "knowledge-a", "向量 检索 与 RAG", 1),
	}); err != nil {
		t.Fatal(err)
	}
	hits, err := store.Search(context.Background(), domainknowledge.KeywordQuery{Text: `向量 "RAG"`, TopK: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ChunkID != "chunk-1" {
		t.Fatalf("unexpected quoted/Chinese FTS result: %#v", hits)
	}
}

func TestStoreSupportsConcurrentWriters(t *testing.T) {
	store := openTestStore(t)
	var wait sync.WaitGroup
	for index := range 12 {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			document := keywordDocument(fmt.Sprintf("chunk-%d", index), "knowledge-a", fmt.Sprintf("concurrent token %d", index), int64(index+1))
			if err := store.Upsert(context.Background(), []domainknowledge.KeywordDocument{document}); err != nil {
				t.Errorf("upsert concurrent document: %v", err)
			}
		}()
	}
	wait.Wait()
	hits, err := store.Search(context.Background(), domainknowledge.KeywordQuery{Text: "concurrent", TopK: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 12 {
		t.Fatalf("expected all concurrent documents, got %d", len(hits))
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(Config{Path: filepath.Join(t.TempDir(), "knowledge.db"), MaxOpenConnections: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close keyword store: %v", err)
		}
	})
	return store
}

func keywordDocument(chunkID string, knowledgeBaseID string, text string, syncSequence int64) domainknowledge.KeywordDocument {
	return domainknowledge.KeywordDocument{
		ChunkID: chunkID, KnowledgeBaseID: knowledgeBaseID, DocumentID: "document-1", DocumentVersion: 1, Text: text, SyncSequence: syncSequence,
	}
}
