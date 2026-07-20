package knowledge

import (
	"fmt"
	"strings"
	"time"
)

type SyncOperation string

const (
	SyncOperationUpsert SyncOperation = "upsert"
	SyncOperationDelete SyncOperation = "delete"
)

type SyncChange struct {
	ID              string
	Sequence        int64
	KnowledgeBaseID string
	EntityType      string
	EntityID        string
	Operation       SyncOperation
	EntityVersion   int64
	Deleted         bool
	OccurredAt      time.Time
}

func (change SyncChange) Validate() error {
	if strings.TrimSpace(change.ID) == "" {
		return fmt.Errorf("sync change id is required")
	}
	if change.Sequence < 1 {
		return fmt.Errorf("sync sequence must be positive")
	}
	if strings.TrimSpace(change.KnowledgeBaseID) == "" {
		return fmt.Errorf("knowledge base id is required")
	}
	if strings.TrimSpace(change.EntityType) == "" {
		return fmt.Errorf("entity type is required")
	}
	if strings.TrimSpace(change.EntityID) == "" {
		return fmt.Errorf("entity id is required")
	}
	if change.Operation != SyncOperationUpsert && change.Operation != SyncOperationDelete {
		return fmt.Errorf("unsupported sync operation %q", change.Operation)
	}
	if change.EntityVersion < 1 {
		return fmt.Errorf("entity version must be positive")
	}
	if change.Operation == SyncOperationDelete && !change.Deleted {
		return fmt.Errorf("delete sync operation must set deleted")
	}
	return nil
}
