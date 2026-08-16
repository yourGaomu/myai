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
	job, err := service.Store.GetExtractionJob(ctx, strings.TrimSpace(command.JobID))
	if err != nil {
		return err
	}
	if job.Status == domainmemory.JobSucceeded {
		return nil
	}
	now := service.now()
	job.Status = domainmemory.JobRunning
	job.Attempts++
	job.LastError = ""
	job.CompletedAt = nil
	job.UpdatedAt = now
	if err := service.Store.SaveExtractionJob(ctx, job); err != nil {
		return err
	}
	run, err := service.Runs.GetRun(ctx, job.AgentRunID)
	if err == nil {
		var events []domainagentrun.Event
		events, err = service.Runs.ListEvents(ctx, []string{run.ID})
		if err == nil {
			drafts, extractErr := service.Extractor.Extract(ctx, run, events)
			if extractErr != nil {
				err = extractErr
			} else {
				for index, draft := range drafts {
					candidate := service.candidateFromDraft(draft, run, events, job.ID, index, now)
					if candidateErr := candidate.Validate(); candidateErr != nil {
						err = candidateErr
						break
					}
					if _, candidateErr := service.Store.GetCandidate(ctx, candidate.ID); candidateErr == nil {
						continue
					} else if !errors.Is(candidateErr, memoryport.ErrNotFound) {
						err = candidateErr
						break
					}
					if candidateErr := service.Store.SaveCandidate(ctx, candidate); candidateErr != nil {
						err = candidateErr
						break
					}
				}
			}
		}
	}
	if err != nil {
		job.Status = domainmemory.JobFailed
		job.LastError = err.Error()
		job.UpdatedAt = service.now()
		if saveErr := service.Store.SaveExtractionJob(ctx, job); saveErr != nil {
			return errors.Join(err, fmt.Errorf("save failed memory extraction job %q: %w", job.ID, saveErr))
		}
		return err
	}
	completedAt := service.now()
	job.Status = domainmemory.JobSucceeded
	job.LastError = ""
	job.UpdatedAt = completedAt
	job.CompletedAt = &completedAt
	return service.Store.SaveExtractionJob(ctx, job)
}

func (service Service) AgentRunCompleted(ctx context.Context, run domainagentrun.Run) {
	if run.Status != domainagentrun.StatusSucceeded && run.Status != domainagentrun.StatusFailed {
		return
	}
	if _, err := service.EnqueueRun(context.WithoutCancel(ctx), memorycommand.EnqueueRun{AgentRunID: run.ID}); err != nil {
		service.report(fmt.Errorf("enqueue memory extraction for agent run %q: %w", run.ID, err))
	}
}

func (service Service) Recover(ctx context.Context, command memorycommand.Recover) error {
	if err := service.validate(); err != nil {
		return err
	}
	limit := command.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	jobs, err := service.Store.ListExtractionJobs(ctx, []domainmemory.JobStatus{
		domainmemory.JobPending, domainmemory.JobRunning, domainmemory.JobFailed,
	}, limit)
	if err != nil {
		return err
	}
	var recoveryErrors []error
	for _, job := range jobs {
		if job.Attempts >= maxAutomaticExtractionAttempts {
			if job.Status != domainmemory.JobFailed {
				job.Status = domainmemory.JobFailed
				if strings.TrimSpace(job.LastError) == "" {
					job.LastError = "automatic retry limit reached after interrupted extraction"
				}
				job.UpdatedAt = service.now()
				if saveErr := service.Store.SaveExtractionJob(ctx, job); saveErr != nil {
					recoveryErrors = append(recoveryErrors, saveErr)
				}
			}
			continue
		}
		if job.Status == domainmemory.JobRunning {
			job.Status = domainmemory.JobPending
			job.UpdatedAt = service.now()
			if saveErr := service.Store.SaveExtractionJob(ctx, job); saveErr != nil {
				recoveryErrors = append(recoveryErrors, saveErr)
				continue
			}
		}
		if service.Async == nil {
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
	if err := service.Store.SaveExtractionJob(ctx, job); err != nil {
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
