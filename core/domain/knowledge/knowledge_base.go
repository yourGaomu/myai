package knowledge

import (
	"fmt"
	"strings"
	"time"
)

type KnowledgeBase struct {
	ID                    string
	CategoryID            string
	Name                  string
	Description           string
	RAGEnabled            bool
	ActiveIndexProfileID  string
	PendingIndexProfileID string
	Deletion              Deletion
	SyncSequence          int64
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (base KnowledgeBase) Validate() error {
	if strings.TrimSpace(base.ID) == "" {
		return fmt.Errorf("knowledge base id is required")
	}
	if strings.TrimSpace(base.Name) == "" {
		return fmt.Errorf("knowledge base name is required")
	}
	if base.RAGEnabled && strings.TrimSpace(base.ActiveIndexProfileID) == "" {
		return fmt.Errorf("active index profile id is required when RAG is enabled")
	}
	if base.SyncSequence < 0 {
		return fmt.Errorf("sync sequence must not be negative")
	}
	if err := base.Deletion.Validate(); err != nil {
		return fmt.Errorf("invalid knowledge base deletion: %w", err)
	}
	return nil
}
