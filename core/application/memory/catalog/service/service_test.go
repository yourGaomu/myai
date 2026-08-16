package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	memoryrepository "myai/core/adapter/persistence/memory/memory"
	"myai/core/application/memory/catalog/command"
	domainmemory "myai/core/domain/memory"
	memoryport "myai/core/port/memory"
)

func TestCatalogLifecyclePreservesVersionsAndLogicalDeletion(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	store := memoryrepository.New()
	service := CatalogService{Store: store, IDs: &sequenceIDs{}, Now: func() time.Time { return now }}

	created, err := service.Create(ctx, command.Create{
		Title: " Agent retry policy ", Kind: domainmemory.KindExperience,
		Tags:       []string{"Go", "go", " retry "},
		Content:    domainmemory.Content{Goal: "Avoid retry loops", Approach: "Persist attempt counters"},
		Confidence: 0.8,
	})
	if err != nil {
		t.Fatalf("create memory: %v", err)
	}
	if created.Memory.CurrentVersion != 1 || !created.Memory.HumanLocked {
		t.Fatalf("unexpected created memory: %#v", created.Memory)
	}
	if len(created.Memory.Tags) != 2 || created.Memory.Tags[0] != "go" || created.Memory.Tags[1] != "retry" {
		t.Fatalf("tags were not normalized: %#v", created.Memory.Tags)
	}

	updated, err := service.Update(ctx, command.Update{
		MemoryID: created.Memory.ID, Title: "Agent retry policy", Kind: domainmemory.KindExperience,
		Tags:       []string{"go", "recovery"},
		Content:    domainmemory.Content{Goal: "Avoid retry loops", Approach: "Stop automatic retries after three attempts"},
		Confidence: 0.95,
	})
	if err != nil {
		t.Fatalf("update memory: %v", err)
	}
	if updated.Memory.CurrentVersion != 2 || len(updated.Memory.Revisions) != 2 {
		t.Fatalf("update did not append a revision: %#v", updated.Memory)
	}
	if updated.Memory.Revisions[0].Content.Approach != "Persist attempt counters" {
		t.Fatalf("update overwrote revision history: %#v", updated.Memory.Revisions)
	}

	if err := service.Delete(ctx, command.Delete{MemoryID: created.Memory.ID, Reason: "replace later"}); err != nil {
		t.Fatalf("delete memory: %v", err)
	}
	active, err := service.List(ctx, command.List{})
	if err != nil {
		t.Fatalf("list active memories: %v", err)
	}
	if len(active.Memories) != 0 {
		t.Fatalf("deleted memory leaked into active list: %#v", active.Memories)
	}
	deleted, err := service.List(ctx, command.List{Filter: memoryport.ListFilter{IncludeDeleted: true}})
	if err != nil || len(deleted.Memories) != 1 || deleted.Memories[0].Status != domainmemory.StatusDeleted {
		t.Fatalf("list deleted memories: memories=%#v err=%v", deleted.Memories, err)
	}

	restored, err := service.Restore(ctx, command.Restore{MemoryID: created.Memory.ID})
	if err != nil {
		t.Fatalf("restore memory: %v", err)
	}
	if restored.Memory.Status != domainmemory.StatusActive || restored.Memory.DeletedAt != nil {
		t.Fatalf("unexpected restored memory: %#v", restored.Memory)
	}
}

func TestCatalogCandidateApprovalAndRejection(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	store := memoryrepository.New()
	service := CatalogService{Store: store, IDs: &sequenceIDs{}, Now: func() time.Time { return now }}

	approvedCandidate, err := service.CreateCandidate(ctx, command.CreateCandidate{
		Title: "Use durable jobs", Kind: domainmemory.KindDecision, Scope: domainmemory.Scope{Type: domainmemory.ScopeGlobal},
		Content:    domainmemory.Content{Goal: "Recover background extraction", Lessons: "Persist before scheduling"},
		Confidence: 0.85, Sources: []domainmemory.SourceRef{{Type: domainmemory.SourceAgentRun, AgentRunID: "run-1", CreatedAt: now}},
	})
	if err != nil {
		t.Fatalf("create candidate: %v", err)
	}
	approved, err := service.ApproveCandidate(ctx, command.ApproveCandidate{CandidateID: approvedCandidate.MemoryCandidate.ID, HumanApprove: true})
	if err != nil {
		t.Fatalf("approve candidate: %v", err)
	}
	if !approved.Memory.HumanLocked || approved.Memory.CurrentVersion != 1 {
		t.Fatalf("unexpected approved memory: %#v", approved.Memory)
	}
	storedApproved, err := store.GetCandidate(ctx, approvedCandidate.MemoryCandidate.ID)
	if err != nil || storedApproved.Status != domainmemory.CandidateApproved || storedApproved.TargetMemoryID != approved.Memory.ID {
		t.Fatalf("candidate approval was not persisted: candidate=%#v err=%v", storedApproved, err)
	}

	rejectedCandidate, err := service.CreateCandidate(ctx, command.CreateCandidate{
		Title: "Unsafe shortcut", Kind: domainmemory.KindFailure, Scope: domainmemory.Scope{Type: domainmemory.ScopeGlobal},
		Content: domainmemory.Content{Goal: "Skip validation", Lessons: "Do not skip validation"}, Confidence: 0.7,
	})
	if err != nil {
		t.Fatalf("create rejected candidate: %v", err)
	}
	rejected, err := service.RejectCandidate(ctx, command.RejectCandidate{CandidateID: rejectedCandidate.MemoryCandidate.ID, Note: "not reusable"})
	if err != nil {
		t.Fatalf("reject candidate: %v", err)
	}
	if rejected.MemoryCandidate.Status != domainmemory.CandidateRejected || rejected.MemoryCandidate.ReviewNote != "not reusable" {
		t.Fatalf("unexpected rejected candidate: %#v", rejected.MemoryCandidate)
	}
}

func TestRecordUseIsAtomicAcrossConcurrentCalls(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	store := memoryrepository.New()
	service := CatalogService{Store: store, IDs: &sequenceIDs{}, Now: func() time.Time { return now }}
	created, err := service.Create(ctx, command.Create{
		Title: "Atomic usage", Kind: domainmemory.KindExperience,
		Content: domainmemory.Content{Goal: "Count concurrent retrieval", Approach: "Use repository atomic update"},
	})
	if err != nil {
		t.Fatalf("create memory: %v", err)
	}
	const calls = 100
	var wait sync.WaitGroup
	wait.Add(calls)
	errorsFound := make(chan error, calls)
	for range calls {
		go func() {
			defer wait.Done()
			if recordErr := service.RecordUse(ctx, command.RecordUse{MemoryID: created.Memory.ID}); recordErr != nil {
				errorsFound <- recordErr
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for recordErr := range errorsFound {
		t.Fatalf("record use: %v", recordErr)
	}
	stored, err := store.Get(ctx, created.Memory.ID)
	if err != nil {
		t.Fatalf("load memory: %v", err)
	}
	if stored.UseCount != calls || stored.LastUsedAt == nil || !stored.LastUsedAt.Equal(now) {
		t.Fatalf("unexpected usage state: %#v", stored)
	}
}

type sequenceIDs struct{ next int }

func (ids *sequenceIDs) NewID() string {
	ids.next++
	return fmt.Sprintf("id-%d", ids.next)
}
