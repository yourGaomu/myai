package memory

import (
	"context"
	"errors"

	domainmemory "myai/core/domain/memory"
)

var (
	ErrNotFound = errors.New("memory not found")
	ErrConflict = errors.New("memory conflict")
)

type Repository interface {
	Get(ctx context.Context, memoryID string) (domainmemory.Memory, error)
	List(ctx context.Context, filter ListFilter) ([]domainmemory.Memory, error)
	Save(ctx context.Context, memory domainmemory.Memory) error
}

type CandidateRepository interface {
	GetCandidate(ctx context.Context, candidateID string) (domainmemory.Candidate, error)
	ListCandidates(ctx context.Context, filter CandidateFilter) ([]domainmemory.Candidate, error)
	SaveCandidate(ctx context.Context, candidate domainmemory.Candidate) error
}

type ExtractionJobRepository interface {
	GetExtractionJob(ctx context.Context, jobID string) (domainmemory.ExtractionJob, error)
	GetExtractionJobByRun(ctx context.Context, agentRunID string, extractorVersion string) (domainmemory.ExtractionJob, error)
	ListExtractionJobs(ctx context.Context, statuses []domainmemory.JobStatus, limit int) ([]domainmemory.ExtractionJob, error)
	SaveExtractionJob(ctx context.Context, job domainmemory.ExtractionJob) error
}

type DreamRunRepository interface {
	GetDreamRun(ctx context.Context, runID string) (domainmemory.DreamRun, error)
	ListDreamRuns(ctx context.Context, limit int) ([]domainmemory.DreamRun, error)
	SaveDreamRun(ctx context.Context, run domainmemory.DreamRun) error
}

type Store interface {
	Repository
	CandidateRepository
	ExtractionJobRepository
	DreamRunRepository
}

type IDGenerator interface {
	NewID() string
}
