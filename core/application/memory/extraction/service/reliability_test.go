package service

import (
	"context"
	"errors"
	"fmt"
	agentrunrepository "myai/core/adapter/persistence/memory/agentrun"
	memoryrepository "myai/core/adapter/persistence/memory/memory"
	memorycommand "myai/core/application/memory/extraction/command"
	domainagentrun "myai/core/domain/agentrun"
	domainmemory "myai/core/domain/memory"
	memoryport "myai/core/port/memory"
	"sync/atomic"
	"testing"
	"time"
)

type blockingExtractor struct {
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

type simultaneousReadStore struct {
	memoryport.Store
	reads atomic.Int32
	gate  chan struct{}
}

func (s *simultaneousReadStore) GetExtractionJob(ctx context.Context, id string) (domainmemory.ExtractionJob, error) {
	job, err := s.Store.GetExtractionJob(ctx, id)
	if s.reads.Add(1) == 2 {
		close(s.gate)
	}
	select {
	case <-s.gate:
		return job, err
	case <-ctx.Done():
		return job, ctx.Err()
	}
}

func (*blockingExtractor) Version() string { return "test-v1" }
func (e *blockingExtractor) Extract(ctx context.Context, _ domainagentrun.Run, _ []domainagentrun.Event) ([]domainmemory.CandidateDraft, error) {
	e.calls.Add(1)
	select {
	case e.started <- struct{}{}:
	default:
	}
	select {
	case <-e.release:
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func extractionFixture(t *testing.T, extractor memoryport.CandidateExtractor) (Service, domainmemory.ExtractionJob) {
	t.Helper()
	now := time.Now()
	store := memoryrepository.New()
	runs := agentrunrepository.New()
	run := terminalRun(now)
	if err := runs.SaveRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	job := domainmemory.ExtractionJob{ID: "job", AgentRunID: run.ID, ExtractorVersion: "test-v1", Status: domainmemory.JobPending, CreatedAt: now, UpdatedAt: now}
	if err := store.SaveExtractionJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	return Service{Store: store, Runs: runs, Extractor: extractor, IDs: &testIDs{}}, job
}

func TestConcurrentProcessClaimsOnlyOnce(t *testing.T) {
	extractor := &blockingExtractor{started: make(chan struct{}, 2), release: make(chan struct{})}
	service, job := extractionFixture(t, extractor)
	service.Store = &simultaneousReadStore{Store: service.Store, gate: make(chan struct{})}
	done := make(chan error, 2)
	go func() { done <- service.Process(context.Background(), memorycommand.Process{JobID: job.ID}) }()
	go func() { done <- service.Process(context.Background(), memorycommand.Process{JobID: job.ID}) }()
	select {
	case <-extractor.started:
	case <-time.After(5 * time.Second):
		t.Fatal("extractor did not start")
	}
	if err := service.Process(context.Background(), memorycommand.Process{JobID: job.ID}); err != nil {
		t.Fatal(err)
	}
	close(extractor.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if extractor.calls.Load() != 1 {
		t.Fatal("duplicate model requests")
	}
}

type failSecondCandidateStore struct {
	memoryport.Store
	writes int
	failed bool
}

func (s *failSecondCandidateStore) InsertCandidateIfAbsent(ctx context.Context, candidate domainmemory.Candidate) error {
	s.writes++
	if s.writes == 2 && !s.failed {
		s.failed = true
		return errors.New("temporary storage outage")
	}
	return s.Store.InsertCandidateIfAbsent(ctx, candidate)
}

func TestRetryUsesPersistedBatchAndPreservesReviewedCandidate(t *testing.T) {
	draft := domainmemory.CandidateDraft{Title: "original", Kind: domainmemory.KindExperience, Scope: domainmemory.Scope{Type: domainmemory.ScopeGlobal}, Content: domainmemory.Content{Goal: "original goal", Approach: "original approach"}, Confidence: 0.9}
	extractor := &stubExtractor{drafts: []domainmemory.CandidateDraft{draft, draft}}
	service, job := extractionFixture(t, extractor)
	store := &failSecondCandidateStore{Store: service.Store}
	service.Store = store
	if err := service.Process(context.Background(), memorycommand.Process{JobID: job.ID}); err == nil {
		t.Fatal("expected partial persistence failure")
	}
	stored, _ := store.GetExtractionJob(context.Background(), job.ID)
	if !stored.ResultPrepared || stored.Status != domainmemory.JobFailed {
		t.Fatalf("batch was not durably saved: %#v", stored)
	}
	first, _ := store.GetCandidate(context.Background(), stored.Candidates[0].ID)
	first.ReviewNote = "human reviewed"
	if err := store.SaveCandidate(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	extractor.drafts = nil // a second model request would lose the original second candidate
	if _, err := service.Retry(context.Background(), memorycommand.Retry{JobID: job.ID}); err != nil {
		t.Fatal(err)
	}
	if err := service.Process(context.Background(), memorycommand.Process{JobID: job.ID}); err != nil {
		t.Fatal(err)
	}
	candidates, _ := store.ListCandidates(context.Background(), memoryport.CandidateFilter{})
	if len(candidates) != 2 || extractor.calls != 1 {
		t.Fatalf("batch regenerated or lost: count=%d calls=%d", len(candidates), extractor.calls)
	}
	first, _ = store.GetCandidate(context.Background(), first.ID)
	if first.ReviewNote != "human reviewed" {
		t.Fatal("retry overwrote reviewed candidate")
	}
}

func TestRecoverSkipsExhaustedJobsBeforeLimitAndLiveLeases(t *testing.T) {
	service, job := extractionFixture(t, &stubExtractor{})
	ctx := context.Background()
	for i := 0; i < 110; i++ {
		failed := job
		failed.ID, failed.AgentRunID = fmt.Sprintf("failed-%d", i), fmt.Sprintf("failed-run-%d", i)
		failed.Status, failed.Attempts, failed.UpdatedAt = domainmemory.JobFailed, 3, job.UpdatedAt.Add(-time.Hour)
		if err := service.Store.SaveExtractionJob(ctx, failed); err != nil {
			t.Fatal(err)
		}
	}
	live := job
	live.ID, live.AgentRunID, live.Status = "live", "live-run", domainmemory.JobRunning
	lease := time.Now().Add(time.Hour)
	live.LeaseUntil = &lease
	if err := service.Store.SaveExtractionJob(ctx, live); err != nil {
		t.Fatal(err)
	}
	executor := &recordingExecutor{}
	service.Async = executor
	if err := service.Recover(ctx, memorycommand.Recover{Limit: 1}); err != nil {
		t.Fatal(err)
	}
	if len(executor.tasks) != 1 {
		t.Fatalf("recoverable job starved: %d", len(executor.tasks))
	}
	liveAfter, _ := service.Store.GetExtractionJob(ctx, live.ID)
	if liveAfter.Status != domainmemory.JobRunning || liveAfter.Revision != 0 {
		t.Fatal("live lease was stolen")
	}
}

func TestInvalidCandidateRejectsWholeBatchBeforeAnyWrites(t *testing.T) {
	draft := domainmemory.CandidateDraft{Title: "valid", Kind: domainmemory.KindExperience, Scope: domainmemory.Scope{Type: domainmemory.ScopeGlobal}, Content: domainmemory.Content{Goal: "goal", Approach: "approach"}, Confidence: 0.9}
	extractor := &stubExtractor{drafts: []domainmemory.CandidateDraft{draft, {}}}
	service, job := extractionFixture(t, extractor)
	if err := service.Process(context.Background(), memorycommand.Process{JobID: job.ID}); err == nil {
		t.Fatal("invalid batch accepted")
	}
	candidates, err := service.Store.ListCandidates(context.Background(), memoryport.CandidateFilter{})
	if err != nil || len(candidates) != 0 {
		t.Fatalf("invalid batch was partially published: %#v %v", candidates, err)
	}
}
