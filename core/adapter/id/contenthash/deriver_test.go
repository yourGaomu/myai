package contenthash

import (
	"testing"

	domainknowledge "myai/core/domain/knowledge"
)

func TestChunkIDIsStable(t *testing.T) {
	identity := domainknowledge.ChunkIdentity{
		DocumentID:        "document-1",
		DocumentVersion:   2,
		ParsingProfileID:  "parsing-v1",
		ChunkingProfileID: "markdown-v1",
		Ordinal:           3,
		ContentHash:       "content-hash",
	}
	deriver := Deriver{}

	first := deriver.ChunkID(identity)
	second := deriver.ChunkID(identity)
	if first != second {
		t.Fatalf("stable chunk id changed: %q != %q", first, second)
	}
}

func TestChunkIDChangesWithDocumentVersion(t *testing.T) {
	identity := domainknowledge.ChunkIdentity{
		DocumentID:        "document-1",
		DocumentVersion:   1,
		ParsingProfileID:  "parsing-v1",
		ChunkingProfileID: "markdown-v1",
		Ordinal:           0,
		ContentHash:       "content-hash",
	}
	deriver := Deriver{}

	first := deriver.ChunkID(identity)
	identity.DocumentVersion++
	second := deriver.ChunkID(identity)
	if first == second {
		t.Fatal("different document versions must not share a chunk id")
	}
}

func TestLengthPrefixPreventsPartBoundaryCollisions(t *testing.T) {
	deriver := Deriver{}
	first := deriver.EmbeddingID("ab", "c")
	second := deriver.EmbeddingID("a", "bc")
	if first == second {
		t.Fatal("different id parts must not collide")
	}
}
