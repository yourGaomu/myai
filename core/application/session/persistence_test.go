package sessionapp

import (
	"testing"
	"time"

	"myai/core/llm"
	repository "myai/core/port/repository"
	"myai/core/session"
)

func TestBuildSessionRecordUsesCurrentSessionState(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	current := &session.Session{
		ID:                "session-1",
		Model:             "gpt-5",
		AgentMode:         session.AgentModePlan,
		PermissionMode:    session.PermissionModeFull,
		ContextWindowK:    16,
		Summary:           "summary",
		CompactedMessages: 2,
		Usage:             llm.TokenUsage{TotalTokens: 9, Available: true},
	}

	record := BuildSessionRecord(BuildSessionRecordCommand{
		SessionID:    "session-1",
		Model:        "",
		Title:        "",
		DefaultModel: "fallback",
		Current:      current,
		Now:          now,
	})

	if record.Model != "fallback" || record.Title != DefaultTitle {
		t.Fatalf("unexpected defaults: %#v", record)
	}
	if record.AgentMode != string(session.AgentModePlan) || record.PermissionMode != string(session.PermissionModeFull) {
		t.Fatalf("unexpected modes: %#v", record)
	}
	if record.CompactedAt == nil || !record.CompactedAt.Equal(now) {
		t.Fatalf("expected compacted time from now: %#v", record.CompactedAt)
	}
	if record.Usage == nil || record.Usage.TotalTokens != 9 {
		t.Fatalf("unexpected usage: %#v", record.Usage)
	}
}

func TestBuildSessionRecordFallsBackToExistingRecord(t *testing.T) {
	existing := repository.SessionRecord{
		ID:             "session-1",
		AgentMode:      string(session.AgentModePlan),
		PermissionMode: string(session.PermissionModeReadonly),
		ContextWindowK: 8,
		Summary:        "old summary",
	}

	record := BuildSessionRecord(BuildSessionRecordCommand{
		SessionID:    "session-1",
		DefaultModel: "fallback",
		Existing:     existing,
		HasExisting:  true,
	})

	if record.AgentMode != existing.AgentMode || record.PermissionMode != existing.PermissionMode {
		t.Fatalf("expected existing modes: %#v", record)
	}
	if record.Summary != existing.Summary || record.ContextWindowK != existing.ContextWindowK {
		t.Fatalf("expected existing state: %#v", record)
	}
}

func TestPrepareSessionRecordForSavePreservesExistingTitle(t *testing.T) {
	createdAt := time.Date(2026, 7, 1, 1, 0, 0, 0, time.UTC)
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)

	record, err := PrepareSessionRecordForSave(PrepareSessionRecordCommand{
		Record: repository.SessionRecord{
			ID:    "session-1",
			Model: "gpt-5",
			Title: DefaultTitle,
		},
		Existing: repository.SessionRecord{
			Title:     "original title",
			CreatedAt: createdAt,
		},
		HasExisting: true,
		Now:         now,
	})
	if err != nil {
		t.Fatal(err)
	}

	if record.Title != "original title" {
		t.Fatalf("expected existing title, got %q", record.Title)
	}
	if !record.CreatedAt.Equal(createdAt) || !record.UpdatedAt.Equal(now) {
		t.Fatalf("unexpected timestamps: %#v", record)
	}
}
