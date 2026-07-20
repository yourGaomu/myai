package knowledge

import (
	"fmt"
	"strings"
	"time"
)

type VectorIndexDefinition struct {
	EmbeddingProfileID string
	Dimensions         int
	DistanceMetricID   string
	Options            map[string]string
}

type VectorDeletion struct {
	EmbeddingProfileID string
	EmbeddingIDs       []string
	SyncSequence       int64
	DeletedAt          time.Time
}

func (definition VectorIndexDefinition) Validate() error {
	if strings.TrimSpace(definition.EmbeddingProfileID) == "" {
		return fmt.Errorf("vector index embedding profile id is required")
	}
	if definition.Dimensions < 1 {
		return fmt.Errorf("vector index dimensions must be positive")
	}
	if strings.TrimSpace(definition.DistanceMetricID) == "" {
		return fmt.Errorf("vector index distance metric id is required")
	}
	for key := range definition.Options {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("vector index option key must not be empty")
		}
	}
	return nil
}

func (deletion VectorDeletion) Validate() error {
	if strings.TrimSpace(deletion.EmbeddingProfileID) == "" {
		return fmt.Errorf("vector deletion embedding profile id is required")
	}
	if len(deletion.EmbeddingIDs) == 0 {
		return fmt.Errorf("vector deletion embedding ids are required")
	}
	for _, embeddingID := range deletion.EmbeddingIDs {
		if strings.TrimSpace(embeddingID) == "" {
			return fmt.Errorf("vector deletion embedding id must not be empty")
		}
	}
	if deletion.SyncSequence < 0 {
		return fmt.Errorf("vector deletion sync sequence must not be negative")
	}
	return nil
}
