package agentrun

import (
	"errors"
	"strings"
	"time"
)

type Status string

const (
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusPaused    Status = "paused"
	StatusCanceled  Status = "canceled"
)

type Kind string

const (
	KindChat       Kind = "chat"
	KindRegenerate Kind = "regenerate"
	KindPlan       Kind = "plan"
	KindInternal   Kind = "internal"
)

type Run struct {
	ID           string
	RequestID    string
	SessionID    string
	Kind         Kind
	Title        string
	Reason       string
	Status       Status
	CurrentStep  int
	TotalSteps   int
	LastSequence int64
	ErrorMessage string
	StartedAt    time.Time
	FinishedAt   *time.Time
}

func (r Run) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return errors.New("agent run id is empty")
	}
	if strings.TrimSpace(r.SessionID) == "" {
		return errors.New("agent run session id is empty")
	}
	if r.StartedAt.IsZero() {
		return errors.New("agent run started at is empty")
	}
	if r.CurrentStep < 0 || r.TotalSteps < 0 || r.CurrentStep > r.TotalSteps && r.TotalSteps > 0 {
		return errors.New("agent run step progress is invalid")
	}
	return nil
}

func (r Run) IsTerminal() bool {
	switch r.Status {
	case StatusSucceeded, StatusFailed, StatusPaused, StatusCanceled:
		return true
	default:
		return false
	}
}
