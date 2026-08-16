package service

import (
	"context"
	"errors"
	"testing"
	"time"

	agentrunrepository "myai/core/adapter/persistence/memory/agentrun"
	memoryrepository "myai/core/adapter/persistence/memory/memory"
	memorycommand "myai/core/application/memory/extraction/command"
	domainagentrun "myai/core/domain/agentrun"
	domainmemory "myai/core/domain/memory"
	memoryport "myai/core/port/memory"
)

func TestProcessCreatesCandidatesOnceAndCompletesJob(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	store := memoryrepository.New()
	runs := agentrunrepository.New()
	run := terminalRun(now)
	if err := runs.SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	job := domainmemory.ExtractionJob{
		ID: "job-1", AgentRunID: run.ID, ExtractorVersion: "test-v1",
		Status: domainmemory.JobPending, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.SaveExtractionJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	extractor := &stubExtractor{drafts: []domainmemory.CandidateDraft{{
		Title: "Durable jobs", Kind: domainmemory.KindExperience, Scope: domainmemory.Scope{Type: domainmemory.ScopeGlobal},
		Content: domainmemory.Content{Goal: "Recover extraction", Approach: "Persist before scheduling"}, Confidence: 0.9,
	}}}
	service := Service{Store: store, Runs: runs, Extractor: extractor, IDs: &testIDs{}, Now: func() time.Time { return now }}

	if err := service.Process(ctx, memorycommand.Process{JobID: job.ID}); err != nil {
		t.Fatalf("process job: %v", err)
	}
	if err := service.Process(ctx, memorycommand.Process{JobID: job.ID}); err != nil {
		t.Fatalf("process completed job again: %v", err)
	}
	storedJob, err := store.GetExtractionJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("load job: %v", err)
	}
	if storedJob.Status != domainmemory.JobSucceeded || storedJob.Attempts != 1 || storedJob.CompletedAt == nil {
		t.Fatalf("unexpected completed job: %#v", storedJob)
	}
	candidates, err := store.ListCandidates(ctx, memoryport.CandidateFilter{})
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].OriginKey != "job-1:0" {
		t.Fatalf("expected one deterministic candidate, got %#v", candidates)
	}
	if len(candidates[0].Sources) != 1 || candidates[0].Sources[0].AgentRunID != run.ID {
		t.Fatalf("expected AgentRun source, got %#v", candidates[0].Sources)
	}
	if extractor.calls != 1 {
		t.Fatalf("completed job was extracted more than once: %d", extractor.calls)
	}
}

func TestEnqueueReturnsExistingJobAfterUniqueConflict(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	runs := agentrunrepository.New()
	run := terminalRun(now)
	if err := runs.SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	existing := domainmemory.ExtractionJob{
		ID: "existing-job", AgentRunID: run.ID, ExtractorVersion: "test-v1",
		Status: domainmemory.JobPending, CreatedAt: now, UpdatedAt: now,
	}
	store := &conflictOnCreateStore{Store: memoryrepository.New(), existing: existing}
	service := Service{Store: store, Runs: runs, Extractor: &stubExtractor{}, IDs: &testIDs{}, Now: func() time.Time { return now }}

	result, err := service.EnqueueRun(ctx, memorycommand.EnqueueRun{AgentRunID: run.ID})
	if err != nil {
		t.Fatalf("enqueue run: %v", err)
	}
	if result.ExtractionJob.ID != existing.ID || store.lookups != 2 {
		t.Fatalf("expected existing job after conflict, got job=%#v lookups=%d", result.ExtractionJob, store.lookups)
	}
}

func TestRecoverStopsAutomaticallyRetryingAfterThreeAttempts(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	store := memoryrepository.New()
	jobs := []domainmemory.ExtractionJob{
		{ID: "retryable", AgentRunID: "run-1", ExtractorVersion: "test-v1", Status: domainmemory.JobFailed, Attempts: 2, LastError: "temporary", CreatedAt: now, UpdatedAt: now},
		{ID: "exhausted", AgentRunID: "run-2", ExtractorVersion: "test-v1", Status: domainmemory.JobFailed, Attempts: 3, LastError: "permanent", CreatedAt: now, UpdatedAt: now},
		{ID: "interrupted", AgentRunID: "run-3", ExtractorVersion: "test-v1", Status: domainmemory.JobRunning, Attempts: 3, CreatedAt: now, UpdatedAt: now},
	}
	for _, job := range jobs {
		if err := store.SaveExtractionJob(ctx, job); err != nil {
			t.Fatalf("save job %s: %v", job.ID, err)
		}
	}
	executor := &recordingExecutor{}
	service := Service{
		Store: store, Runs: agentrunrepository.New(), Extractor: &stubExtractor{}, IDs: &testIDs{},
		Async: executor, Now: func() time.Time { return now.Add(time.Minute) },
	}

	if err := service.Recover(ctx, memorycommand.Recover{}); err != nil {
		t.Fatalf("recover jobs: %v", err)
	}
	if len(executor.tasks) != 1 {
		t.Fatalf("expected only the retryable job to be submitted, got %d tasks", len(executor.tasks))
	}
	interrupted, err := store.GetExtractionJob(ctx, "interrupted")
	if err != nil {
		t.Fatalf("load interrupted job: %v", err)
	}
	if interrupted.Status != domainmemory.JobFailed || interrupted.LastError == "" {
		t.Fatalf("interrupted exhausted job was not finalized: %#v", interrupted)
	}
}

func TestRetryResubmitsFailedJobBeyondAutomaticLimit(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	store := memoryrepository.New()
	job := domainmemory.ExtractionJob{
		ID: "failed-job", AgentRunID: "run-1", ExtractorVersion: "test-v1",
		Status: domainmemory.JobFailed, Attempts: 3, LastError: "model timeout", CreatedAt: now, UpdatedAt: now,
	}
	if err := store.SaveExtractionJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	executor := &recordingExecutor{}
	service := Service{
		Store: store, Runs: agentrunrepository.New(), Extractor: &stubExtractor{}, IDs: &testIDs{},
		Async: executor, Now: func() time.Time { return now.Add(time.Minute) },
	}

	retried, err := service.Retry(ctx, memorycommand.Retry{JobID: job.ID})
	if err != nil {
		t.Fatalf("retry failed job: %v", err)
	}
	if retried.ExtractionJob.Status != domainmemory.JobPending || retried.ExtractionJob.Attempts != 3 || retried.ExtractionJob.LastError != "" {
		t.Fatalf("unexpected retried job: %#v", retried.ExtractionJob)
	}
	if len(executor.tasks) != 1 {
		t.Fatalf("expected one retry task, got %d", len(executor.tasks))
	}
	failed, err := service.ListJobs(ctx, memorycommand.ListJobs{Statuses: []domainmemory.JobStatus{domainmemory.JobFailed}})
	if err != nil {
		t.Fatalf("list failed jobs: %v", err)
	}
	if len(failed.Items) != 0 {
		t.Fatalf("retried job remained failed: %#v", failed.Items)
	}
}

func TestRetryRejectsNonFailedJob(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	store := memoryrepository.New()
	job := domainmemory.ExtractionJob{
		ID: "pending-job", AgentRunID: "run-1", ExtractorVersion: "test-v1",
		Status: domainmemory.JobPending, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.SaveExtractionJob(ctx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	service := Service{Store: store, Runs: agentrunrepository.New(), Extractor: &stubExtractor{}, IDs: &testIDs{}}
	if _, err := service.Retry(ctx, memorycommand.Retry{JobID: job.ID}); err == nil {
		t.Fatal("expected retry of pending job to fail")
	}
}

type stubExtractor struct {
	drafts []domainmemory.CandidateDraft
	err    error
	calls  int
}

func (extractor *stubExtractor) Version() string { return "test-v1" }

func (extractor *stubExtractor) Extract(context.Context, domainagentrun.Run, []domainagentrun.Event) ([]domainmemory.CandidateDraft, error) {
	extractor.calls++
	return extractor.drafts, extractor.err
}

type conflictOnCreateStore struct {
	memoryport.Store
	existing domainmemory.ExtractionJob
	lookups  int
}

func (store *conflictOnCreateStore) GetExtractionJobByRun(context.Context, string, string) (domainmemory.ExtractionJob, error) {
	store.lookups++
	if store.lookups == 1 {
		return domainmemory.ExtractionJob{}, memoryport.ErrNotFound
	}
	return store.existing, nil
}

func (store *conflictOnCreateStore) SaveExtractionJob(context.Context, domainmemory.ExtractionJob) error {
	return memoryport.ErrConflict
}

type recordingExecutor struct{ tasks []func() }

func (executor *recordingExecutor) Submit(task func()) error {
	if task == nil {
		return errors.New("task is nil")
	}
	executor.tasks = append(executor.tasks, task)
	return nil
}

type testIDs struct{ next int }

func (ids *testIDs) NewID() string {
	ids.next++
	return "generated-id"
}

func terminalRun(now time.Time) domainagentrun.Run {
	finishedAt := now.Add(time.Minute)
	return domainagentrun.Run{
		ID: "run-1", SessionID: "session-1", Kind: domainagentrun.KindChat,
		Status: domainagentrun.StatusSucceeded, StartedAt: now, FinishedAt: &finishedAt,
	}
}
