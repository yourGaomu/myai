package mapper

import (
	"testing"
	"time"

	domainknowledge "myai/core/domain/knowledge"
)

func TestKnowledgeBaseRoundTrip(t *testing.T) {
	deletedAt := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	base := domainknowledge.KnowledgeBase{
		ID:                    "knowledge-1",
		Name:                  "MyAI",
		Description:           "Project documentation",
		RAGEnabled:            true,
		ActiveIndexProfileID:  "index-1",
		PendingIndexProfileID: "index-2",
		Deletion: domainknowledge.Deletion{
			Deleted:      true,
			DeletedAt:    &deletedAt,
			DeleteReason: "archived",
		},
		SyncSequence: 9,
		CreatedAt:    deletedAt.Add(-time.Hour),
		UpdatedAt:    deletedAt,
	}

	document := KnowledgeBaseDocumentFromDomain(base)
	mapped := KnowledgeBaseDomainFromDocument(document)
	if mapped.ID != base.ID || mapped.ActiveIndexProfileID != base.ActiveIndexProfileID || !mapped.Deletion.Deleted {
		t.Fatalf("unexpected knowledge base round trip: %#v", mapped)
	}
	if mapped.Deletion.DeletedAt == base.Deletion.DeletedAt {
		t.Fatal("mapper must clone deletion timestamp pointers")
	}
}

func TestKnowledgeDocumentRoundTrip(t *testing.T) {
	now := time.Date(2026, 7, 19, 11, 0, 0, 0, time.UTC)
	document := domainknowledge.Document{
		ID:              "document-1",
		KnowledgeBaseID: "knowledge-1",
		FileName:        "guide.md",
		ContentType:     "text/markdown",
		ObjectKey:       "knowledge/knowledge-1/document-1/v1/guide.md",
		ContentHash:     "sha256",
		Version:         1,
		Status:          domainknowledge.DocumentStatusReady,
		SyncSequence:    3,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	persistent := KnowledgeDocumentFromDomain(document)
	mapped := KnowledgeDocumentDomainFromDocument(persistent)
	if mapped.ID != document.ID || mapped.Status != domainknowledge.DocumentStatusReady || mapped.ObjectKey != document.ObjectKey {
		t.Fatalf("unexpected knowledge document round trip: %#v", mapped)
	}
}
