package toolapp

import (
	"testing"
	"time"

	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
)

func TestToolCallEntry(t *testing.T) {
	createdAt := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	entry := ToolCallEntry(ToolCallEntryCommand{
		SessionID: "session-1",
		Call: domainmessage.ToolCall{
			ID:        "call-1",
			Name:      "read_file",
			Arguments: `{"path":"README.md"}`,
		},
		CreatedAt: createdAt,
	})

	if entry.Kind != domaintool.ExecutionEntryToolCall || entry.ToolCallID != "call-1" || entry.ToolName != "read_file" {
		t.Fatalf("unexpected tool call entry: %#v", entry)
	}
	if !entry.CreatedAt.Equal(createdAt) {
		t.Fatalf("unexpected created time: %#v", entry.CreatedAt)
	}
}

func TestToolResultEntry(t *testing.T) {
	entry := ToolResultEntry(ToolResultEntryCommand{
		SessionID: "session-1",
		Call: domainmessage.ToolCall{
			ID:        "call-1",
			Name:      "read_file",
			Arguments: `{"path":"README.md"}`,
		},
		Result:    "content",
		ToolError: "boom",
	})

	if entry.Kind != domaintool.ExecutionEntryToolResult || entry.Content != "content" || entry.Error != "boom" {
		t.Fatalf("unexpected tool result entry: %#v", entry)
	}
}

func TestToolResultMessage(t *testing.T) {
	message := ToolResultMessage(domainmessage.ToolCall{
		ID:   "call-1",
		Name: "read_file",
	}, "content")

	result, ok := message.FirstToolResult()
	if !ok {
		t.Fatal("expected tool result")
	}
	if result.ToolCallID != "call-1" || result.Name != "read_file" || result.Content != "content" {
		t.Fatalf("unexpected tool result: %#v", result)
	}
}
