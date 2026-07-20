package knowledge

import (
	"fmt"
	"strings"
	"time"
)

type IndexProfileStatus string

const (
	IndexProfileStatusBuilding IndexProfileStatus = "building"
	IndexProfileStatusActive   IndexProfileStatus = "active"
	IndexProfileStatusFailed   IndexProfileStatus = "failed"
	IndexProfileStatusRetired  IndexProfileStatus = "retired"
)

type EmbeddingModelInfo struct {
	ID             string
	Name           string
	Provider       string
	Model          string
	ModelVersion   string
	Dimensions     int
	MaxInputTokens int
	Enabled        bool
}

type EmbeddingProfile struct {
	ID           string
	Name         string
	ModelID      string
	Provider     string
	Model        string
	ModelVersion string
	Dimensions   int
	Normalize    bool
	InputMode    string
	Deletion     Deletion
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ParsingProfile struct {
	ID            string
	Name          string
	ParserID      string
	ParserVersion string
	OCR           bool
	Options       map[string]string
	Deletion      Deletion
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type ChunkingProfile struct {
	ID              string
	Name            string
	StrategyID      string
	StrategyVersion string
	MaxChunkSize    int
	Overlap         int
	Tokenizer       string
	Options         map[string]string
	Deletion        Deletion
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type IndexProfile struct {
	ID                 string
	Name               string
	ParsingProfileID   string
	ChunkingProfileID  string
	EmbeddingProfileID string
	DistanceMetricID   string
	VectorIndexOptions map[string]string
	Status             IndexProfileStatus
	FailureReason      string
	Deletion           Deletion
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (profile ParsingProfile) Validate() error {
	if strings.TrimSpace(profile.ID) == "" {
		return fmt.Errorf("parsing profile id is required")
	}
	if strings.TrimSpace(profile.Name) == "" {
		return fmt.Errorf("parsing profile name is required")
	}
	if strings.TrimSpace(profile.ParserID) == "" {
		return fmt.Errorf("parser id is required")
	}
	if strings.TrimSpace(profile.ParserVersion) == "" {
		return fmt.Errorf("parser version is required")
	}
	if err := profile.Deletion.Validate(); err != nil {
		return fmt.Errorf("invalid parsing profile deletion: %w", err)
	}
	return nil
}

func (info EmbeddingModelInfo) Validate() error {
	if strings.TrimSpace(info.ID) == "" {
		return fmt.Errorf("embedding model id is required")
	}
	if strings.TrimSpace(info.Name) == "" {
		return fmt.Errorf("embedding model name is required")
	}
	if strings.TrimSpace(info.Provider) == "" {
		return fmt.Errorf("embedding model provider is required")
	}
	if strings.TrimSpace(info.Model) == "" {
		return fmt.Errorf("embedding model is required")
	}
	if info.Dimensions < 1 {
		return fmt.Errorf("embedding dimensions must be positive")
	}
	if info.MaxInputTokens < 0 {
		return fmt.Errorf("max input tokens must not be negative")
	}
	return nil
}

func (profile EmbeddingProfile) Validate() error {
	if strings.TrimSpace(profile.ID) == "" {
		return fmt.Errorf("embedding profile id is required")
	}
	if strings.TrimSpace(profile.Name) == "" {
		return fmt.Errorf("embedding profile name is required")
	}
	if strings.TrimSpace(profile.ModelID) == "" {
		return fmt.Errorf("embedding model id is required")
	}
	if strings.TrimSpace(profile.Provider) == "" {
		return fmt.Errorf("embedding provider is required")
	}
	if strings.TrimSpace(profile.Model) == "" {
		return fmt.Errorf("embedding model is required")
	}
	if profile.Dimensions < 1 {
		return fmt.Errorf("embedding dimensions must be positive")
	}
	if err := profile.Deletion.Validate(); err != nil {
		return fmt.Errorf("invalid embedding profile deletion: %w", err)
	}
	return nil
}

func (profile ChunkingProfile) Validate() error {
	if strings.TrimSpace(profile.ID) == "" {
		return fmt.Errorf("chunking profile id is required")
	}
	if strings.TrimSpace(profile.Name) == "" {
		return fmt.Errorf("chunking profile name is required")
	}
	if strings.TrimSpace(profile.StrategyID) == "" {
		return fmt.Errorf("chunking strategy id is required")
	}
	if strings.TrimSpace(profile.StrategyVersion) == "" {
		return fmt.Errorf("chunking strategy version is required")
	}
	if profile.MaxChunkSize < 1 {
		return fmt.Errorf("max chunk size must be positive")
	}
	if profile.Overlap < 0 || profile.Overlap >= profile.MaxChunkSize {
		return fmt.Errorf("chunk overlap must be between 0 and max chunk size - 1")
	}
	if err := profile.Deletion.Validate(); err != nil {
		return fmt.Errorf("invalid chunking profile deletion: %w", err)
	}
	return nil
}

func (profile IndexProfile) Validate() error {
	if strings.TrimSpace(profile.ID) == "" {
		return fmt.Errorf("index profile id is required")
	}
	if strings.TrimSpace(profile.Name) == "" {
		return fmt.Errorf("index profile name is required")
	}
	if strings.TrimSpace(profile.ParsingProfileID) == "" {
		return fmt.Errorf("parsing profile id is required")
	}
	if strings.TrimSpace(profile.ChunkingProfileID) == "" {
		return fmt.Errorf("chunking profile id is required")
	}
	if strings.TrimSpace(profile.EmbeddingProfileID) == "" {
		return fmt.Errorf("embedding profile id is required")
	}
	if strings.TrimSpace(profile.DistanceMetricID) == "" {
		return fmt.Errorf("distance metric id is required")
	}
	if !IsIndexProfileStatus(profile.Status) {
		return fmt.Errorf("unsupported index profile status %q", profile.Status)
	}
	if profile.Status == IndexProfileStatusFailed && strings.TrimSpace(profile.FailureReason) == "" {
		return fmt.Errorf("failure reason is required for failed index profile")
	}
	if err := profile.Deletion.Validate(); err != nil {
		return fmt.Errorf("invalid index profile deletion: %w", err)
	}
	return nil
}

func IsIndexProfileStatus(status IndexProfileStatus) bool {
	switch status {
	case IndexProfileStatusBuilding,
		IndexProfileStatusActive,
		IndexProfileStatusFailed,
		IndexProfileStatusRetired:
		return true
	default:
		return false
	}
}
