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
