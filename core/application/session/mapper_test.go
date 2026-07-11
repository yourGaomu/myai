package sessionapp

import (
	"testing"
	"time"

	domainmessage "myai/core/domain/message"
	repository "myai/core/port/repository"
)

func TestMessagesAfterID(t *testing.T) {
	now := time.Now()
	records := []repository.MessageRecord{
		{ID: "1", CreatedAt: now},
		{ID: "2", CreatedAt: now},
		{ID: "3", CreatedAt: now},
	}

	got, truncated, err := MessagesAfterID(records, "1", 2)
	if err != nil || truncated || len(got) != 2 || got[0].ID != "2" {
		t.Fatalf("unexpected result: %#v %v %v", got, truncated, err)
	}
}

func TestMessagesFromRecordsAddsSystemPrompt(t *testing.T) {
	records := []repository.MessageRecord{
		{Role: repository.RoleUser, Content: "hello"},
	}

	messages := MessagesFromRecords(records)
	if len(messages) != 2 {
		t.Fatalf("unexpected messages length: %d", len(messages))
	}
	if messages[0].Role != domainmessage.RoleSystem {
		t.Fatalf("expected system prompt first, got %s", messages[0].Role)
	}
}
