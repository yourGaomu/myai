package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	memoryrepository "myai/core/adapter/persistence/memory/memory"
	memorycatalogcommand "myai/core/application/memory/catalog/command"
	memorycatalogservice "myai/core/application/memory/catalog/service"
	memorydreamcommand "myai/core/application/memory/dream/command"
	domainmemory "myai/core/domain/memory"
)

func TestDreamRunAppliesSafeActionsAndProtectsHumanMemory(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	store := memoryrepository.New()
	ids := &sequenceIDs{}
	catalog := memorycatalogservice.CatalogService{Store: store, IDs: ids, Now: func() time.Time { return now }}

	locked, err := catalog.Create(ctx, memorycatalogcommand.Create{
		Title: "Human rule", Kind: domainmemory.KindDecision,
		Content: domainmemory.Content{Goal: "Protect manual decisions", Lessons: "Do not overwrite people"},
	})
	if err != nil {
		t.Fatalf("create locked memory: %v", err)
	}
	unlockedSeed := createCandidate(t, ctx, catalog, "Unlocked target")
	unlocked, err := catalog.ApproveCandidate(ctx, memorycatalogcommand.ApproveCandidate{CandidateID: unlockedSeed.ID})
	if err != nil {
		t.Fatalf("create unlocked target: %v", err)
	}

	create := createCandidate(t, ctx, catalog, "Create candidate")
	merge := createCandidate(t, ctx, catalog, "Merge candidate")
	reject := createCandidate(t, ctx, catalog, "Reject candidate")
	protected := createCandidate(t, ctx, catalog, "Protected candidate")
	consolidator := staticConsolidator{actions: []domainmemory.DreamAction{
		{CandidateID: create.ID, Decision: domainmemory.DecisionCreate, Reason: "new reusable lesson"},
		{CandidateID: merge.ID, MemoryID: unlocked.Memory.ID, Decision: domainmemory.DecisionMerge, Reason: "same goal"},
		{CandidateID: reject.ID, Decision: domainmemory.DecisionReject, Reason: "not reusable"},
		{CandidateID: protected.ID, MemoryID: locked.Memory.ID, Decision: domainmemory.DecisionMerge, Reason: "looks related"},
	}}
	dream := &Service{Store: store, Catalog: catalog, Consolidator: consolidator, IDs: ids, Now: func() time.Time { return now }}

	result, err := dream.Run(ctx, memorydreamcommand.Run{Trigger: "manual"})
	if err != nil {
		t.Fatalf("run dream: %v", err)
	}
	run := result.DreamRun
	if run.Status != domainmemory.DreamSucceeded || run.CreatedCount != 1 || run.MergedCount != 1 || run.RejectedCount != 1 {
		t.Fatalf("unexpected dream counters: %#v", run)
	}
	if len(run.Actions) != 4 || run.Actions[3].Applied || run.Actions[3].FailureReason == "" {
		t.Fatalf("human-locked merge was not audited: %#v", run.Actions)
	}
	if run.Actions[0].CandidateTitle != create.Title || run.Actions[1].MemoryTitle != unlocked.Memory.Title {
		t.Fatalf("dream action titles were not recorded: %#v", run.Actions)
	}
	assertCandidateStatus(t, ctx, store, create.ID, domainmemory.CandidateApproved)
	assertCandidateStatus(t, ctx, store, merge.ID, domainmemory.CandidateMerged)
	assertCandidateStatus(t, ctx, store, reject.ID, domainmemory.CandidateRejected)
	assertCandidateStatus(t, ctx, store, protected.ID, domainmemory.CandidatePending)

	updatedTarget, err := store.Get(ctx, unlocked.Memory.ID)
	if err != nil || updatedTarget.CurrentVersion != 2 || updatedTarget.HumanLocked {
		t.Fatalf("unexpected merged target: memory=%#v err=%v", updatedTarget, err)
	}
}

func createCandidate(t *testing.T, ctx context.Context, catalog memorycatalogservice.CatalogService, title string) domainmemory.Candidate {
	t.Helper()
	created, err := catalog.CreateCandidate(ctx, memorycatalogcommand.CreateCandidate{
		Title: title, Kind: domainmemory.KindExperience, Scope: domainmemory.Scope{Type: domainmemory.ScopeGlobal},
		Content: domainmemory.Content{Goal: title + " goal", Lessons: title + " lesson"}, Confidence: 0.8,
	})
	if err != nil {
		t.Fatalf("create candidate %q: %v", title, err)
	}
	return created.MemoryCandidate
}

func assertCandidateStatus(t *testing.T, ctx context.Context, store *memoryrepository.Repository, candidateID string, status domainmemory.CandidateStatus) {
	t.Helper()
	candidate, err := store.GetCandidate(ctx, candidateID)
	if err != nil || candidate.Status != status {
		t.Fatalf("unexpected candidate status: candidate=%#v err=%v", candidate, err)
	}
}

type staticConsolidator struct {
	actions []domainmemory.DreamAction
}

func (consolidator staticConsolidator) Consolidate(context.Context, []domainmemory.Candidate, []domainmemory.Memory) ([]domainmemory.DreamAction, error) {
	return append([]domainmemory.DreamAction(nil), consolidator.actions...), nil
}

type sequenceIDs struct {
	next int
}

func (ids *sequenceIDs) NewID() string {
	ids.next++
	return fmt.Sprintf("id-%d", ids.next)
}
