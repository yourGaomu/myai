package knowledge

import (
	"fmt"
	"strings"
)

type ChunkDraft struct {
	Ordinal       int
	Text          string
	ContentHash   string
	StartOffset   int
	EndOffset     int
	SourcePage    int
	SourceHeading string
}

type ObjectRef struct {
	ObjectKey   string
	ContentHash string
	Size        int64
	ContentType string
}

type ObjectInfo struct {
	ObjectKey   string
	ContentHash string
	Size        int64
	ContentType string
}

type DocumentSourceRequest struct {
	URL      string
	Code     string
	MaxBytes int64
}

type DocumentSourceContent struct {
	FileName    string
	ContentType string
	Size        int64
	Data        []byte
}

func (content DocumentSourceContent) Validate() error {
	if strings.TrimSpace(content.FileName) == "" {
		return fmt.Errorf("document source file name is required")
	}
	if content.Size < 0 || int64(len(content.Data)) != content.Size {
		return fmt.Errorf("document source size does not match content")
	}
	return nil
}

func (draft ChunkDraft) Validate() error {
	if draft.Ordinal < 0 {
		return fmt.Errorf("chunk ordinal must not be negative")
	}
	if strings.TrimSpace(draft.Text) == "" {
		return fmt.Errorf("chunk text is required")
	}
	if strings.TrimSpace(draft.ContentHash) == "" {
		return fmt.Errorf("chunk content hash is required")
	}
	if draft.StartOffset < 0 || draft.EndOffset < draft.StartOffset {
		return fmt.Errorf("chunk offsets are invalid")
	}
	if draft.SourcePage < 0 {
		return fmt.Errorf("source page must not be negative")
	}
	return nil
}
