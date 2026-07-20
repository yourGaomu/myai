package knowledge

import (
	"fmt"
	"strings"
	"time"
)

const MaxCategoryDepth = 5

type KnowledgeCategory struct {
	ID          string
	Name        string
	ParentID    string
	AncestorIDs []string
	SortOrder   int
	Deletion    Deletion
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (category KnowledgeCategory) Validate() error {
	if strings.TrimSpace(category.ID) == "" {
		return fmt.Errorf("knowledge category id is required")
	}
	if strings.TrimSpace(category.Name) == "" {
		return fmt.Errorf("knowledge category name is required")
	}
	if len(category.AncestorIDs) >= MaxCategoryDepth {
		return fmt.Errorf("knowledge category depth must not exceed %d", MaxCategoryDepth)
	}
	seen := make(map[string]struct{}, len(category.AncestorIDs))
	for _, ancestorID := range category.AncestorIDs {
		ancestorID = strings.TrimSpace(ancestorID)
		if ancestorID == "" {
			return fmt.Errorf("knowledge category ancestor id must not be empty")
		}
		if ancestorID == category.ID {
			return fmt.Errorf("knowledge category cannot contain itself as an ancestor")
		}
		if _, exists := seen[ancestorID]; exists {
			return fmt.Errorf("knowledge category ancestor ids must be unique")
		}
		seen[ancestorID] = struct{}{}
	}
	if category.ParentID == "" && len(category.AncestorIDs) != 0 {
		return fmt.Errorf("root knowledge category must not contain ancestors")
	}
	if category.ParentID != "" {
		if len(category.AncestorIDs) == 0 || category.AncestorIDs[len(category.AncestorIDs)-1] != category.ParentID {
			return fmt.Errorf("knowledge category parent must be the last ancestor")
		}
	}
	if err := category.Deletion.Validate(); err != nil {
		return fmt.Errorf("invalid knowledge category deletion: %w", err)
	}
	return nil
}
