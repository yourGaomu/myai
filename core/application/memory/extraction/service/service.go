package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"myai/core/application/memory/extraction/api"
	memorycommand "myai/core/application/memory/extraction/command"
	"myai/core/application/memory/extraction/result"
	domainagentrun "myai/core/domain/agentrun"
	domainmemory "myai/core/domain/memory"
	agentrunport "myai/core/port/agentrun"
	asyncport "myai/core/port/async"
	memoryport "myai/core/port/memory"
)

const (
	defaultExtractorVersion        = "memory-extractor-v1"
	maxAutomaticExtractionAttempts = 3
)

type Service struct {
	Store     memoryport.Store
	Runs      agentrunport.Repository
	Extractor memoryport.CandidateExtractor
	IDs       memoryport.IDGenerator
	Async     asyncport.Executor
	Now       func() time.Time
	OnError   func(error)
}

var _ api.Service = Service{}

// RunRecovery also recovers leases that were still live during startup and
// retries durable pending jobs left behind by a full worker queue.
func (service Service) RunRecovery(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := service.Recover(ctx, memorycommand.Recover{Limit: 100}); err != nil {
				service.report(err)
			}
		}
	}
}

func (service Service) EnqueueRun(ctx context.Context, command memorycommand.EnqueueRun) (result.Job, error) {
	if err := service.validate(); err != nil {
		return result.Job{}, err
	}
	runID := strings.TrimSpace(command.AgentRunID)
	if runID == "" {
		return result.Job{}, errors.New("agent run id is empty")
	}
	if _, err := service.Runs.GetRun(ctx, runID); err != nil {
		return result.Job{}, err
	}
	version := service.extractorVersion()
	if existing, err := service.Store.GetExtractionJobByRun(ctx, runID, version); err == nil {
		return result.Job{ExtractionJob: existing}, nil
	} else if !errors.Is(err, memoryport.ErrNotFound) {
		return result.Job{}, err
	}
	now := service.now()
	job := domainmemory.ExtractionJob{
		ID: service.IDs.NewID(), AgentRunID: runID, ExtractorVersion: version,
		Status: domainmemory.JobPending, CreatedAt: now, UpdatedAt: now,
	}
	if err := job.Validate(); err != nil {
		return result.Job{}, err
	}
	if err := service.Store.SaveExtractionJob(ctx, job); err != nil {
		if errors.Is(err, memoryport.ErrConflict) {
			existing, loadErr := service.Store.GetExtractionJobByRun(ctx, runID, version)
			if loadErr == nil {
				return result.Job{ExtractionJob: existing}, nil
			}
			return result.Job{}, errors.Join(err, loadErr)
		}
		return result.Job{}, err
	}
	if service.Async != nil {
		if err := service.Async.Submit(func() {
			if processErr := service.Process(context.Background(), memorycommand.Process{JobID: job.ID}); processErr != nil {
				service.report(processErr)
			}
		}); err != nil {
			// The durable pending job remains recoverable after the worker queue is available.
			service.report(fmt.Errorf("submit memory extraction job %q: %w", job.ID, err))
		}
	}
	return result.Job{ExtractionJob: job}, nil
}

func (service Service) ListJobs(ctx context.Context, command memorycommand.ListJobs) (result.Jobs, error) {
	if err := service.validate(); err != nil {
		return result.Jobs{}, err
	}
	limit := command.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	jobs, err := service.Store.ListExtractionJobs(ctx, command.Statuses, limit)
	if err != nil {
		return result.Jobs{}, err
	}
	return result.Jobs{Items: jobs}, nil
}

func (service Service) Process(ctx context.Context, command memorycommand.Process) error {
	if err := service.validate(); err != nil {
		return err
	}
	// Bound work below the lease duration. A crashed worker is recoverable;
	// a slow/stale worker cannot overwrite a newer owner's job state.
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	job, err := service.Store.GetExtractionJob(ctx, strings.TrimSpace(command.JobID))
	if err != nil {
		return err
	}
	if job.Status == domainmemory.JobSucceeded {
		return nil
	}
	if job.Status == domainmemory.JobRunning && job.LeaseUntil != nil && job.LeaseUntil.After(service.now()) {
		return nil
	}
	if job.Status != domainmemory.JobPending && job.Attempts >= maxAutomaticExtractionAttempts {
		return nil
	}
	expectedRevision := job.Revision
	now := service.now()
	job.Status = domainmemory.JobRunning
	job.Attempts++
	job.Revision++
	lease := now.Add(5 * time.Minute)
	job.LeaseUntil = &lease
	job.LastError = ""
	job.CompletedAt = nil
	job.UpdatedAt = now
	if err := service.Store.CompareAndSwapExtractionJob(ctx, expectedRevision, job); err != nil {
		if errors.Is(err, memoryport.ErrConflict) {
			return nil
		}
		return err
	}
	if !job.ResultPrepared {
		prepared, prepareErr := service.prepareCandidates(ctx, job, now)
		if prepareErr != nil {
			return service.finishJob(ctx, job, prepareErr)
		}
		if err := ctx.Err(); err != nil {
			return service.finishJob(ctx, job, err)
		}
		job.Candidates, job.ResultPrepared = prepared, true
		job.Revision++
		if err := service.Store.CompareAndSwapExtractionJob(ctx, job.Revision-1, job); err != nil {
			return err
		}
	}
	return service.finishJob(ctx, job, service.persistPreparedCandidates(ctx, job))
}

func (service Service) prepareCandidates(ctx context.Context, job domainmemory.ExtractionJob, now time.Time) ([]domainmemory.Candidate, error) {
	run, err := service.Runs.GetRun(ctx, job.AgentRunID)
	if err != nil {
		return nil, err
	}
	events, err := service.Runs.ListEvents(ctx, []string{run.ID})
	if err != nil {
		return nil, err
	}
	drafts, err := service.Extractor.Extract(ctx, run, events)
	if err != nil {
		return nil, err
	}
	prepared := make([]domainmemory.Candidate, 0, len(drafts))
	for index, draft := range drafts {
		candidate := service.candidateFromDraft(draft, run, events, job.ID, index, now)
		if err := candidate.Validate(); err != nil {
			return nil, err
		}
		prepared = append(prepared, candidate)
	}
	return prepared, nil
}

func (service Service) persistPreparedCandidates(ctx context.Context, job domainmemory.ExtractionJob) error {
	for _, candidate := range job.Candidates {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := service.Store.InsertCandidateIfAbsent(ctx, candidate); err != nil {
			return err
		}
	}
	return nil
}

func (service Service) finishJob(ctx context.Context, job domainmemory.ExtractionJob, cause error) error {
	job.Status = domainmemory.JobSucceeded
	now := service.now()
	job.CompletedAt, job.UpdatedAt, job.LeaseUntil = &now, now, nil
	if cause != nil {
		job.Status, job.LastError, job.CompletedAt = domainmemory.JobFailed, cause.Error(), nil
	}
	job.Revision++
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return errors.Join(cause, service.Store.CompareAndSwapExtractionJob(finishCtx, job.Revision-1, job))
}

func (service Service) AgentRunCompleted(ctx context.Context, run domainagentrun.Run) {
	if run.Status != domainagentrun.StatusSucceeded && run.Status != domainagentrun.StatusFailed {
		return
	}
	if _, err := service.EnqueueRun(context.WithoutCancel(ctx), memorycommand.EnqueueRun{AgentRunID: run.ID}); err != nil {
		service.report(fmt.Errorf("enqueue memory extraction for agent run %q: %w", run.ID, err))
	}
}

// 它用于恢复上次程序异常退出时留下的任务。
func (service Service) Recover(ctx context.Context, command memorycommand.Recover) error {
	if err := service.validate(); err != nil {
		return err
	}
	limit := command.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	//查询还有哪些任务在运行，限制最多100条
	jobs, err := service.Store.ListRecoverableExtractionJobs(ctx, service.now(), maxAutomaticExtractionAttempts, limit)
	if err != nil {
		return err
	}
	var recoveryErrors []error
	for _, job := range jobs {
		exhausted := job.Status != domainmemory.JobPending && job.Attempts >= maxAutomaticExtractionAttempts
		if service.Async == nil && !exhausted {
			continue
		}
		job.Status, job.LeaseUntil = domainmemory.JobPending, nil
		if exhausted {
			job.Status, job.LastError = domainmemory.JobFailed, "automatic retry limit reached after interrupted extraction"
		}
		job.Revision++
		job.UpdatedAt = service.now()
		if saveErr := service.Store.CompareAndSwapExtractionJob(ctx, job.Revision-1, job); saveErr != nil {
			if !errors.Is(saveErr, memoryport.ErrConflict) {
				recoveryErrors = append(recoveryErrors, saveErr)
			}
			continue
		}
		if exhausted {
			continue
		}
		jobID := job.ID
		if submitErr := service.Async.Submit(func() {
			if processErr := service.Process(context.Background(), memorycommand.Process{JobID: jobID}); processErr != nil {
				service.report(processErr)
			}
		}); submitErr != nil {
			recoveryErrors = append(recoveryErrors, submitErr)
		}
	}
	return errors.Join(recoveryErrors...)
}

func (service Service) Retry(ctx context.Context, command memorycommand.Retry) (result.Job, error) {
	if err := service.validate(); err != nil {
		return result.Job{}, err
	}
	jobID := strings.TrimSpace(command.JobID)
	if jobID == "" {
		return result.Job{}, errors.New("memory extraction job id is empty")
	}
	job, err := service.Store.GetExtractionJob(ctx, jobID)
	if err != nil {
		return result.Job{}, err
	}
	if job.Status != domainmemory.JobFailed {
		return result.Job{}, fmt.Errorf("memory extraction job %q is %s, only failed jobs can be retried", job.ID, job.Status)
	}
	job.Status = domainmemory.JobPending
	job.LastError = ""
	job.CompletedAt = nil
	job.UpdatedAt = service.now()
	job.LeaseUntil = nil
	job.Revision++
	if err := service.Store.CompareAndSwapExtractionJob(ctx, job.Revision-1, job); err != nil {
		return result.Job{}, err
	}
	if service.Async != nil {
		if err := service.Async.Submit(func() {
			if processErr := service.Process(context.Background(), memorycommand.Process{JobID: job.ID}); processErr != nil {
				service.report(processErr)
			}
		}); err != nil {
			return result.Job{ExtractionJob: job}, fmt.Errorf("submit memory extraction retry %q: %w", job.ID, err)
		}
	}
	return result.Job{ExtractionJob: job}, nil
}

func (service Service) candidateFromDraft(draft domainmemory.CandidateDraft, run domainagentrun.Run, events []domainagentrun.Event, jobID string, ordinal int, now time.Time) domainmemory.Candidate {
	sources := append([]domainmemory.SourceRef(nil), draft.Sources...)
	if len(sources) == 0 {
		eventIDs := make([]string, 0, len(events))
		for _, event := range events {
			eventIDs = append(eventIDs, event.ID)
		}
		sources = []domainmemory.SourceRef{{Type: domainmemory.SourceAgentRun, SessionID: run.SessionID, AgentRunID: run.ID, EventIDs: eventIDs, CreatedAt: now}}
	}
	originKey := fmt.Sprintf("%s:%d", jobID, ordinal)
	digest := sha256.Sum256([]byte(originKey))
	return domainmemory.Candidate{
		ID: hex.EncodeToString(digest[:]), OriginKey: originKey, Title: strings.TrimSpace(draft.Title), Kind: draft.Kind,
		Scope: draft.Scope, Tags: draft.Tags, Content: draft.Content, Confidence: draft.Confidence,
		Sources: sources, Status: domainmemory.CandidatePending, CreatedAt: now, UpdatedAt: now,
	}
}

func (service Service) validate() error {
	if service.Store == nil {
		return errors.New("memory store is nil")
	}
	if service.Runs == nil {
		return errors.New("agent run repository is nil")
	}
	if service.Extractor == nil {
		return errors.New("memory candidate extractor is nil")
	}
	if service.IDs == nil {
		return errors.New("memory id generator is nil")
	}
	return nil
}

func (service Service) extractorVersion() string {
	version := strings.TrimSpace(service.Extractor.Version())
	if version == "" {
		return defaultExtractorVersion
	}
	return version
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func (service Service) report(err error) {
	if err != nil && service.OnError != nil {
		service.OnError(err)
	}
}
