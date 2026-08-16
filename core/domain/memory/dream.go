package memory

import (
	"errors"
	"strings"
	"time"
)

type DreamAction struct {
	CandidateID    string
	CandidateTitle string
	MemoryID       string
	MemoryTitle    string
	Decision       DreamDecision
	Reason         string
	Applied        bool
	FailureReason  string
}

type DreamRun struct {
	ID              string
	Status          DreamStatus
	Trigger         string
	CandidateCount  int
	CreatedCount    int
	MergedCount     int
	SupersededCount int
	RejectedCount   int
	Actions         []DreamAction
	LastError       string
	StartedAt       time.Time
	FinishedAt      *time.Time
}

func (run DreamRun) Validate() error {
	if strings.TrimSpace(run.ID) == "" {
		return errors.New("dream run id is empty")
	}
	switch run.Status {
	case DreamPending, DreamRunning, DreamSucceeded, DreamFailed:
	default:
		return errors.New("dream run status is invalid")
	}
	if run.CandidateCount < 0 || run.CreatedCount < 0 || run.MergedCount < 0 || run.SupersededCount < 0 || run.RejectedCount < 0 {
		return errors.New("dream run counters are negative")
	}
	if run.StartedAt.IsZero() {
		return errors.New("dream run started at is empty")
	}
	return nil
}
