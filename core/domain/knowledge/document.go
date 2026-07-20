package knowledge

import (
	"fmt"
	"strings"
	"time"
)

type DocumentStatus string

const (
	DocumentStatusUploaded  DocumentStatus = "uploaded"
	DocumentStatusParsing   DocumentStatus = "parsing"
	DocumentStatusChunking  DocumentStatus = "chunking"
	DocumentStatusEmbedding DocumentStatus = "embedding"
	DocumentStatusIndexing  DocumentStatus = "indexing"
	DocumentStatusReady     DocumentStatus = "ready"
	DocumentStatusFailed    DocumentStatus = "failed"
	DocumentStatusDeleted   DocumentStatus = "deleted"
)

type Document struct {
	ID              string
	KnowledgeBaseID string
	FileName        string
	ContentType     string
	ObjectKey       string
	ContentHash     string
	Version         int64
	Status          DocumentStatus
	FailureReason   string
	Deletion        Deletion
	SyncSequence    int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func IsDocumentStatus(status DocumentStatus) bool {
	switch status {
	case DocumentStatusUploaded,
		DocumentStatusParsing,
		DocumentStatusChunking,
		DocumentStatusEmbedding,
		DocumentStatusIndexing,
		DocumentStatusReady,
		DocumentStatusFailed,
		DocumentStatusDeleted:
		return true
	default:
		return false
	}
}

func (document Document) Validate() error {
	if strings.TrimSpace(document.ID) == "" {
		return fmt.Errorf("document id is required")
	}
	if strings.TrimSpace(document.KnowledgeBaseID) == "" {
		return fmt.Errorf("knowledge base id is required")
	}
	if strings.TrimSpace(document.FileName) == "" {
		return fmt.Errorf("file name is required")
	}
	if strings.TrimSpace(document.ObjectKey) == "" {
		return fmt.Errorf("object key is required")
	}
	if strings.TrimSpace(document.ContentHash) == "" {
		return fmt.Errorf("content hash is required")
	}
	if document.Version < 1 {
		return fmt.Errorf("document version must be at least 1")
	}
	if !IsDocumentStatus(document.Status) {
		return fmt.Errorf("unsupported document status %q", document.Status)
	}
	if document.Status == DocumentStatusFailed && strings.TrimSpace(document.FailureReason) == "" {
		return fmt.Errorf("failure reason is required for failed document")
	}
	if document.Deletion.Deleted != (document.Status == DocumentStatusDeleted) {
		return fmt.Errorf("deleted document status and deletion flag must match")
	}
	if document.SyncSequence < 0 {
		return fmt.Errorf("sync sequence must not be negative")
	}
	if err := document.Deletion.Validate(); err != nil {
		return fmt.Errorf("invalid document deletion: %w", err)
	}
	return nil
}
