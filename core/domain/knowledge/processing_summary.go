package knowledge

import (
	"fmt"
	"strings"
)

// ProcessingSummary describes a completed Parse + Chunk operation without
// retaining every generated chunk in memory.
type ProcessingSummary struct {
	DocumentID              string
	DocumentVersion         int64
	ContentType             string
	ParserID                string
	ParserVersion           string
	ChunkingStrategyID      string
	ChunkingStrategyVersion string
	ChunkCount              int
	Warnings                []string
}

func (summary ProcessingSummary) Validate() error {
	if strings.TrimSpace(summary.DocumentID) == "" {
		return fmt.Errorf("processing summary document id is required")
	}
	if summary.DocumentVersion < 1 {
		return fmt.Errorf("processing summary document version must be at least 1")
	}
	if strings.TrimSpace(summary.ContentType) == "" {
		return fmt.Errorf("processing summary content type is required")
	}
	if strings.TrimSpace(summary.ParserID) == "" || strings.TrimSpace(summary.ParserVersion) == "" {
		return fmt.Errorf("processing summary parser id and version are required")
	}
	if strings.TrimSpace(summary.ChunkingStrategyID) == "" || strings.TrimSpace(summary.ChunkingStrategyVersion) == "" {
		return fmt.Errorf("processing summary chunking strategy id and version are required")
	}
	if summary.ChunkCount < 1 {
		return fmt.Errorf("processing summary chunk count must be positive")
	}
	return nil
}
