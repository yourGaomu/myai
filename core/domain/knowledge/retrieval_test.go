package knowledge

import (
	"math"
	"strings"
	"testing"
)

func TestEmbeddingVectorValidatesDimensions(t *testing.T) {
	embedding := validEmbeddingVector()
	embedding.Dimensions = 3

	err := embedding.Validate()
	if err == nil || !strings.Contains(err.Error(), "dimensions") {
		t.Fatalf("expected dimension validation error, got %v", err)
	}
}

func TestEmbeddingVectorRejectsNonFiniteValues(t *testing.T) {
	embedding := validEmbeddingVector()
	embedding.Values[1] = float32(math.Inf(1))

	err := embedding.Validate()
	if err == nil || !strings.Contains(err.Error(), "finite") {
		t.Fatalf("expected finite value validation error, got %v", err)
	}
}

func TestVectorQueryRejectsNonFiniteValues(t *testing.T) {
	query := VectorQuery{
		EmbeddingProfileID: "embedding-1",
		DistanceMetricID:   "cosine",
		Vector:             []float32{0.1, float32(math.NaN())},
		TopK:               8,
	}

	err := query.Validate()
	if err == nil || !strings.Contains(err.Error(), "finite") {
		t.Fatalf("expected finite value validation error, got %v", err)
	}
}

func TestRetrievalQueryAllowsAllKnowledgeBases(t *testing.T) {
	query := RetrievalQuery{
		Text:               "how does plan mode work",
		IndexProfileID:     "index-1",
		EmbeddingProfileID: "embedding-1",
		TopK:               8,
	}

	if err := query.Validate(); err != nil {
		t.Fatalf("empty knowledge base list should mean all knowledge bases: %v", err)
	}
}

func validEmbeddingVector() EmbeddingVector {
	return EmbeddingVector{
		ID:                 "embedding-1",
		ChunkID:            "chunk-1",
		KnowledgeBaseID:    "knowledge-1",
		DocumentID:         "document-1",
		DocumentVersion:    1,
		EmbeddingProfileID: "profile-1",
		Dimensions:         2,
		Values:             []float32{0.1, 0.2},
	}
}
