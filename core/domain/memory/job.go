package memory

import (
	"errors"
	"strings"
	"time"
)

type ExtractionJob struct {
	ID               string
	AgentRunID       string
	ExtractorVersion string
	Status           JobStatus
	Attempts         int
	LastError        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	CompletedAt      *time.Time
}

func (job ExtractionJob) Validate() error {
	if strings.TrimSpace(job.ID) == "" || strings.TrimSpace(job.AgentRunID) == "" {
		return errors.New("memory extraction job identity is empty")
	}
	if strings.TrimSpace(job.ExtractorVersion) == "" {
		return errors.New("memory extractor version is empty")
	}
	switch job.Status {
	case JobPending, JobRunning, JobSucceeded, JobFailed:
	default:
		return errors.New("memory extraction job status is invalid")
	}
	if job.Attempts < 0 {
		return errors.New("memory extraction job attempts is negative")
	}
	if job.CreatedAt.IsZero() || job.UpdatedAt.IsZero() {
		return errors.New("memory extraction job timestamps are empty")
	}
	return nil
}
