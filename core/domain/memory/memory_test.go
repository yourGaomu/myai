package memory

import (
	"testing"
	"time"
)

func TestMemoryAppendRevisionPreservesHistory(t *testing.T) {
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	memory := validMemory(now)

	err := memory.AppendRevision(Revision{
		ID: "revision-2",
		Content: Content{
			Goal:     "Keep extraction jobs durable",
			Approach: "Persist before submitting to the worker pool",
		},
		Confidence: 0.9,
		Author:     AuthorHuman,
		Sources:    []SourceRef{{Type: SourceManual, CreatedAt: now.Add(time.Minute)}},
		CreatedAt:  now.Add(time.Minute),
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("append revision: %v", err)
	}
	if memory.CurrentVersion != 2 || len(memory.Revisions) != 2 {
		t.Fatalf("expected version 2 with history preserved, got version=%d revisions=%d", memory.CurrentVersion, len(memory.Revisions))
	}
	if memory.Revisions[0].Content.Goal != "Original goal" {
		t.Fatalf("original revision was modified: %#v", memory.Revisions[0])
	}
}

func TestMemoryLogicalDeleteAndRestore(t *testing.T) {
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	memory := validMemory(now)

	if err := memory.MarkDeleted(now.Add(time.Minute), "obsolete"); err != nil {
		t.Fatalf("delete memory: %v", err)
	}
	if memory.Status != StatusDeleted || memory.DeletedAt == nil || memory.DeletionReason != "obsolete" {
		t.Fatalf("unexpected deleted state: %#v", memory)
	}

	if err := memory.Restore(now.Add(2 * time.Minute)); err != nil {
		t.Fatalf("restore memory: %v", err)
	}
	if memory.Status != StatusActive || memory.DeletedAt != nil || memory.DeletionReason != "" {
		t.Fatalf("unexpected restored state: %#v", memory)
	}
}

func TestContentUpdatedAtIgnoresUsageAndFallsBackToCreation(t *testing.T) {
	now := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	memory := validMemory(now.AddDate(-1, 0, 0))
	memory.UpdatedAt = now
	memory.LastUsedAt = &now
	memory.UseCount = 1000
	if got := memory.ContentUpdatedAt(); !got.Equal(memory.CreatedAt) {
		t.Fatalf("legacy usage made content look new: %s", got)
	}
	revisedAt := now.Add(-24 * time.Hour)
	if err := memory.AppendRevision(Revision{
		ID: "revision-2", Content: Content{Goal: "Restore quorum", Approach: "Recover a majority"},
		Confidence: 0.9, Author: AuthorHuman, CreatedAt: revisedAt,
	}, revisedAt); err != nil {
		t.Fatal(err)
	}
	memory.UpdatedAt = now
	if got := memory.ContentUpdatedAt(); !got.Equal(revisedAt) {
		t.Fatalf("expected actual revision time %s, got %s", revisedAt, got)
	}
	for index := range memory.Revisions {
		memory.Revisions[index].CreatedAt = time.Time{}
	}
	if got := memory.ContentUpdatedAt(); !got.Equal(memory.CreatedAt) {
		t.Fatalf("missing legacy revision times must fall back to creation, got %s", got)
	}
}

func validMemory(now time.Time) Memory {
	return Memory{
		ID:             "memory-1",
		Title:          "Durable extraction",
		Kind:           KindExperience,
		Scope:          Scope{Type: ScopeGlobal},
		Status:         StatusActive,
		CurrentVersion: 1,
		Revisions: []Revision{{
			ID:         "revision-1",
			Version:    1,
			Content:    Content{Goal: "Original goal", Approach: "Original approach"},
			Confidence: 0.8,
			Author:     AuthorModel,
			Sources:    []SourceRef{{Type: SourceManual, CreatedAt: now}},
			CreatedAt:  now,
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}
}
