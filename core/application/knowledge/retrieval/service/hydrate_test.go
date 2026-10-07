package service

import (
	"context"
	"errors"
	domainknowledge "myai/core/domain/knowledge"
	"testing"
)

type unavailableDocuments struct{ fakeDocumentRepository }

func (unavailableDocuments) Get(context.Context, string) (domainknowledge.Document, error) {
	return domainknowledge.Document{}, errors.New("document unavailable")
}

func TestHydrateExcludesUnverifiableDocumentsAndStaleChunks(t *testing.T) {
	chunks := []domainknowledge.Chunk{testChunk("current"), testChunk("old")}
	chunks[1].DocumentVersion = 2
	svc := newTestRetrievalService(t, Configuration{LocalVectors: &fakeVectorStore{}}, chunks)
	fused := []domainknowledge.FusedItem{{ChunkID: "current"}, {ChunkID: "old"}}
	hits, _, warnings := svc.hydrate(context.Background(), testRetrievalQuery(2), testProfileRepository().index, fused)
	if len(hits) != 1 || hits[0].ChunkID != "current" || len(warnings) == 0 {
		t.Fatalf("stale cached-document chunk survived: %#v %v", hits, warnings)
	}
	svc.configuration.Documents = unavailableDocuments{}
	hits, selected, warnings := svc.hydrate(context.Background(), testRetrievalQuery(2), testProfileRepository().index, fused)
	if len(hits) != 0 || len(selected) != 0 || len(warnings) != 1 {
		t.Fatalf("unverifiable chunks survived: %#v %v", hits, warnings)
	}
}
