package agentrun

import (
	"context"
	"errors"

	domainagentrun "myai/core/domain/agentrun"
)

var ErrNotFound = errors.New("agent run not found")

type Repository interface {
	SaveRun(ctx context.Context, run domainagentrun.Run) error
	GetRun(ctx context.Context, runID string) (domainagentrun.Run, error)
	ListRunning(ctx context.Context) ([]domainagentrun.Run, error)
	NextEventSequence(ctx context.Context, runID string) (int64, error)
	SaveEvent(ctx context.Context, event domainagentrun.Event) error
	ReplaceEventContent(ctx context.Context, runID string, eventID string, content string, truncated bool) error
	ListRuns(ctx context.Context, sessionID string, limit int) ([]domainagentrun.Run, error)
	ListEvents(ctx context.Context, runIDs []string) ([]domainagentrun.Event, error)
}

type IDGenerator interface {
	NewID() string
}
