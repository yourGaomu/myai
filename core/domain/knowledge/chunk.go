package knowledge

import (
	"fmt"
	"strings"
	"time"
)

type Chunk struct {
	ID                  string
	KnowledgeBaseID     string
	DocumentID          string
	DocumentVersion     int64
	ParsingProfileID    string
	ChunkingProfileID   string
	Ordinal             int
	Text                string
	ContentHash         string
	StartOffset         int
	EndOffset           int
	SourcePage          int
	SourceHeading       string
	EmbeddingProfileIDs []string
	Deletion            Deletion
	SyncSequence        int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type ChunkIdentity struct {
	DocumentID        string
	DocumentVersion   int64
	ParsingProfileID  string
	ChunkingProfileID string
	Ordinal           int
	ContentHash       string
}

func (chunk Chunk) Validate() error {
	if strings.TrimSpace(chunk.ID) == "" {
		return fmt.Errorf("chunk id is required")
	}
	if strings.TrimSpace(chunk.KnowledgeBaseID) == "" {
		return fmt.Errorf("knowledge base id is required")
	}
	identity := ChunkIdentity{
		DocumentID:        chunk.DocumentID,
		DocumentVersion:   chunk.DocumentVersion,
		ParsingProfileID:  chunk.ParsingProfileID,
		ChunkingProfileID: chunk.ChunkingProfileID,
		Ordinal:           chunk.Ordinal,
		ContentHash:       chunk.ContentHash,
	}
	if err := identity.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(chunk.Text) == "" {
		return fmt.Errorf("chunk text is required")
	}
	if chunk.StartOffset < 0 || chunk.EndOffset < chunk.StartOffset {
		return fmt.Errorf("chunk offsets are invalid")
	}
	if chunk.SourcePage < 0 {
		return fmt.Errorf("source page must not be negative")
	}
	seenProfiles := make(map[string]struct{}, len(chunk.EmbeddingProfileIDs))
	for _, profileID := range chunk.EmbeddingProfileIDs {
		profileID = strings.TrimSpace(profileID)
		if profileID == "" {
			return fmt.Errorf("embedding profile id must not be empty")
		}
		if _, exists := seenProfiles[profileID]; exists {
			return fmt.Errorf("embedding profile id %q is duplicated", profileID)
		}
		seenProfiles[profileID] = struct{}{}
	}
	if chunk.SyncSequence < 0 {
		return fmt.Errorf("sync sequence must not be negative")
	}
	if err := chunk.Deletion.Validate(); err != nil {
		return fmt.Errorf("invalid chunk deletion: %w", err)
	}
	return nil
}

func (identity ChunkIdentity) Validate() error {
	if strings.TrimSpace(identity.DocumentID) == "" {
		return fmt.Errorf("document id is required")
	}
	if identity.DocumentVersion < 1 {
		return fmt.Errorf("document version must be at least 1")
	}
	if strings.TrimSpace(identity.ParsingProfileID) == "" {
		return fmt.Errorf("parsing profile id is required")
	}
	if strings.TrimSpace(identity.ChunkingProfileID) == "" {
		return fmt.Errorf("chunking profile id is required")
	}
	if identity.Ordinal < 0 {
		return fmt.Errorf("chunk ordinal must not be negative")
	}
	if strings.TrimSpace(identity.ContentHash) == "" {
		return fmt.Errorf("chunk content hash is required")
	}
	return nil
}
