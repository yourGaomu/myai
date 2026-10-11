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
			Kind:            domaintool.ExecutionEntryToolResult,
			SessionID:       "session-1",
			ToolCallID:      "call-1",
			ToolName:        "read_file",
			Content:         "content",
			Error:           "warning",
			Status:          domaintool.ResultStatusFailed,
			ErrorCode:       "tool_failed",
			Truncated:       true,
			PromptContent:   "bounded content",
			PromptError:     "bounded warning",
			PromptTruncated: true,
			CreatedAt:       createdAt.Add(time.Nanosecond),
		},
		{Kind: "unknown"},
	})

	if len(records) != 2 {
		t.Fatalf("record count = %d, want 2", len(records))
	}
	if records[0].ID != "id-1" || records[0].Role != repository.RoleToolCall || records[0].ToolArguments == "" {
		t.Fatalf("unexpected tool call record: %#v", records[0])
	}
	if records[1].ID != "id-2" || records[1].Role != repository.RoleTool || records[1].Content != "content" || records[1].ToolError != "warning" || records[1].ToolStatus != "failed" || records[1].ToolErrorCode != "tool_failed" || !records[1].ToolTruncated {
		t.Fatalf("unexpected tool result record: %#v", records[1])
	}
	if records[1].ToolPromptContent != "bounded content" || records[1].ToolPromptError != "bounded warning" || !records[1].ToolPromptTruncated {
		t.Fatalf("unexpected prompt-limited tool result record: %#v", records[1])
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
	// 1. 实时通知已经分配身份时，落库必须沿用，不能让前端得到第二个资源。
	identified := mapper.AssetRecords([]domaintool.SharedAsset{{ID: "live-asset", SessionID: "child", ToolCallID: "share-call"}})
	if len(identified) != 1 || identified[0].ID != "live-asset" || identified[0].ToolCallID != "share-call" {
		t.Fatalf("live asset identity changed during persistence: %#v", identified)
	}
}
