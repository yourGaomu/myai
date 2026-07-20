package mapper

import (
	"reflect"
	"testing"
	"time"

	domainknowledge "myai/core/domain/knowledge"
)

func TestChunkRoundTripOwnsNestedEmbeddingProfiles(t *testing.T) {
	now := time.Now().UTC()
	chunk := domainknowledge.Chunk{
		ID:                  "chunk-1",
		KnowledgeBaseID:     "knowledge-1",
		DocumentID:          "document-1",
		DocumentVersion:     1,
		ParsingProfileID:    "parsing-1",
		ChunkingProfileID:   "chunking-1",
		Ordinal:             2,
		Text:                "chunk text",
		ContentHash:         "hash",
		EndOffset:           10,
		EmbeddingProfileIDs: []string{"embedding-a", "embedding-b"},
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	document := ChunkDocumentFromDomain(chunk)
	mapped := ChunkDomainFromDocument(document)
	if !reflect.DeepEqual(mapped, chunk) {
		t.Fatalf("chunk round trip changed value: %#v != %#v", mapped, chunk)
	}
	document.EmbeddingProfileIDs[0] = "changed"
	if chunk.EmbeddingProfileIDs[0] != "embedding-a" {
		t.Fatal("mapper must not share embedding profile slice")
	}
}

func TestProfileMappersOwnMapFields(t *testing.T) {
	parsing := domainknowledge.ParsingProfile{
		ID:            "parsing-1",
		Name:          "Python",
		ParserID:      "python",
		ParserVersion: "1",
		Options:       map[string]string{"language": "zh"},
	}
	parsingDocument := ParsingProfileDocumentFromDomain(parsing)
	parsingDocument.Options["language"] = "en"
	if parsing.Options["language"] != "zh" {
		t.Fatal("mapper must not share parsing profile option maps")
	}

	profile := domainknowledge.ChunkingProfile{
		ID:              "chunking-1",
		Name:            "Markdown",
		StrategyID:      "markdown",
		StrategyVersion: "1",
		MaxChunkSize:    100,
		Options:         map[string]string{"language": "zh"},
	}
	document := ChunkingProfileDocumentFromDomain(profile)
	document.Options["language"] = "en"
	if profile.Options["language"] != "zh" {
		t.Fatal("mapper must not share profile option maps")
	}
}
