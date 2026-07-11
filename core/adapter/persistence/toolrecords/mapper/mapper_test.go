package mapper

import (
	"fmt"
	"testing"
	"time"

	domaintool "myai/core/domain/tool"
	repository "myai/core/port/repository"
)

type sequentialIDs struct {
	next int
}

func (g *sequentialIDs) NewID() string {
	g.next++
	return fmt.Sprintf("id-%d", g.next)
}

func TestMapperConvertsDomainEntriesToMessageRecords(t *testing.T) {
	createdAt := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	mapper := Mapper{IDs: &sequentialIDs{}}

	records := mapper.MessageRecords([]domaintool.ExecutionEntry{
		{
			Kind:       domaintool.ExecutionEntryToolCall,
			SessionID:  "session-1",
			ToolCallID: "call-1",
			ToolName:   "read_file",
			Arguments:  `{"path":"README.md"}`,
			CreatedAt:  createdAt,
		},
		{
			Kind:       domaintool.ExecutionEntryToolResult,
			SessionID:  "session-1",
			ToolCallID: "call-1",
			ToolName:   "read_file",
			Content:    "content",
			Error:      "warning",
			CreatedAt:  createdAt.Add(time.Nanosecond),
		},
		{Kind: "unknown"},
	})

	if len(records) != 2 {
		t.Fatalf("record count = %d, want 2", len(records))
	}
	if records[0].ID != "id-1" || records[0].Role != repository.RoleToolCall || records[0].ToolArguments == "" {
		t.Fatalf("unexpected tool call record: %#v", records[0])
	}
	if records[1].ID != "id-2" || records[1].Role != repository.RoleTool || records[1].Content != "content" || records[1].ToolError != "warning" {
		t.Fatalf("unexpected tool result record: %#v", records[1])
	}
}

func TestMapperConvertsSharedAssetToAssetRecord(t *testing.T) {
	expiresAt := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	mapper := Mapper{IDs: &sequentialIDs{}}

	records := mapper.AssetRecords([]domaintool.SharedAsset{{
		SessionID:  "session-1",
		RequestID:  "request-1",
		ToolCallID: "call-1",
		ToolName:   "share_file",
		LocalPath:  "file.txt",
		ShortURL:   "https://s/abc",
		ShortCode:  "abc",
		ExpiresAt:  &expiresAt,
	}})

	if len(records) != 1 || records[0].ID != "id-1" {
		t.Fatalf("unexpected asset records: %#v", records)
	}
	if records[0].SessionID != "session-1" || records[0].ShortCode != "abc" || records[0].ExpiresAt != &expiresAt {
		t.Fatalf("unexpected asset record: %#v", records[0])
	}
}
