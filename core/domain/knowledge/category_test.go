package knowledge

import "testing"

func TestKnowledgeCategoryValidateRejectsDepthBeyondLimit(t *testing.T) {
	category := KnowledgeCategory{
		ID:          "category-6",
		Name:        "Level 6",
		ParentID:    "category-5",
		AncestorIDs: []string{"category-1", "category-2", "category-3", "category-4", "category-5"},
	}

	if err := category.Validate(); err == nil {
		t.Fatal("expected category depth validation error")
	}
}

func TestKnowledgeCategoryValidateRequiresParentAsLastAncestor(t *testing.T) {
	category := KnowledgeCategory{
		ID:          "category-3",
		Name:        "Level 3",
		ParentID:    "category-2",
		AncestorIDs: []string{"category-2", "category-1"},
	}

	if err := category.Validate(); err == nil {
		t.Fatal("expected invalid ancestor path error")
	}
}
