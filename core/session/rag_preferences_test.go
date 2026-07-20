package session

import "testing"

func TestNormalizeRAGSettingsAppliesDefaultsAndRemovesDuplicateIDs(t *testing.T) {
	settings := NormalizeRAGSettings(RAGSettings{
		KnowledgeBaseIDs: []string{" kb-1 ", "kb-1", ""},
		CategoryIDs:      []string{"category-1", " category-1 "},
	})

	if settings.Mode != RetrievalModeAuto || settings.TopK != DefaultRAGTopK {
		t.Fatalf("unexpected defaults: %#v", settings)
	}
	if len(settings.KnowledgeBaseIDs) != 1 || settings.KnowledgeBaseIDs[0] != "kb-1" {
		t.Fatalf("unexpected knowledge base ids: %#v", settings.KnowledgeBaseIDs)
	}
	if len(settings.CategoryIDs) != 1 || settings.CategoryIDs[0] != "category-1" {
		t.Fatalf("unexpected category ids: %#v", settings.CategoryIDs)
	}
}

func TestValidateRAGSettingsRejectsUnsupportedModeAndInvalidTopK(t *testing.T) {
	if err := ValidateRAGSettings(RAGSettings{Mode: RetrievalMode("unknown"), TopK: 8}); err == nil {
		t.Fatal("expected unsupported mode error")
	}
	if err := ValidateRAGSettings(RAGSettings{Mode: RetrievalModeAuto, TopK: 0}); err != nil {
		t.Fatalf("zero top_k should normalize to default: %v", err)
	}
	if err := ValidateRAGSettings(RAGSettings{Mode: RetrievalModeAuto, TopK: MaxRAGTopK + 1}); err == nil {
		t.Fatal("expected top_k upper-bound error")
	}
}

func TestCloneRAGSettingsDoesNotShareSlices(t *testing.T) {
	original := RAGSettings{Mode: RetrievalModeAlways, TopK: 3, KnowledgeBaseIDs: []string{"kb-1"}}
	clone := CloneRAGSettings(original)
	clone.KnowledgeBaseIDs[0] = "kb-2"

	if original.KnowledgeBaseIDs[0] != "kb-1" {
		t.Fatal("clone must not share knowledge base id storage")
	}
}
