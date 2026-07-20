package knowledge

import (
	"strings"
	"testing"
	"time"
)

func TestEmbeddingProfilesWithSameDimensionsRemainDistinct(t *testing.T) {
	first := EmbeddingProfile{
		ID:         "profile-a",
		Name:       "Model A",
		ModelID:    "model-a",
		Provider:   "provider",
		Model:      "embedding-a",
		Dimensions: 1024,
	}
	second := EmbeddingProfile{
		ID:         "profile-b",
		Name:       "Model B",
		ModelID:    "model-b",
		Provider:   "provider",
		Model:      "embedding-b",
		Dimensions: 1024,
	}

	if err := first.Validate(); err != nil {
		t.Fatalf("validate first profile: %v", err)
	}
	if err := second.Validate(); err != nil {
		t.Fatalf("validate second profile: %v", err)
	}
	if first.ID == second.ID {
		t.Fatal("different embedding models must not share a profile id")
	}
}

func TestChunkingProfileRejectsOverlapAtChunkSize(t *testing.T) {
	profile := ChunkingProfile{
		ID:              "chunking-1",
		Name:            "Markdown",
		StrategyID:      "markdown",
		StrategyVersion: "1",
		MaxChunkSize:    1600,
		Overlap:         1600,
	}

	err := profile.Validate()
	if err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("expected overlap validation error, got %v", err)
	}
}

func TestIndexProfileRequiresFailedReason(t *testing.T) {
	profile := IndexProfile{
		ID:                 "index-1",
		Name:               "Default",
		ParsingProfileID:   "parsing-1",
		ChunkingProfileID:  "chunking-1",
		EmbeddingProfileID: "embedding-1",
		DistanceMetricID:   "cosine",
		Status:             IndexProfileStatusFailed,
	}

	err := profile.Validate()
	if err == nil || !strings.Contains(err.Error(), "failure reason") {
		t.Fatalf("expected failure reason validation error, got %v", err)
	}
}

func TestParsingProfileRequiresVersion(t *testing.T) {
	profile := ParsingProfile{
		ID:       "parsing-1",
		Name:     "Default",
		ParserID: "python",
	}
	if err := profile.Validate(); err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("expected parser version validation error, got %v", err)
	}
}

func TestDeletionRequiresTimestamp(t *testing.T) {
	if err := (Deletion{Deleted: true}).Validate(); err == nil {
		t.Fatal("expected deleted object without timestamp to fail")
	}

	now := time.Now()
	if err := (Deletion{Deleted: true, DeletedAt: &now}).Validate(); err != nil {
		t.Fatalf("validate logical deletion: %v", err)
	}
}
