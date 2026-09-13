package subagent

import "errors"

var (
	ErrSchedulerQueueFull = errors.New("subagent scheduler queue is full")
	ErrSchedulerClosed    = errors.New("subagent scheduler is closed")
)

type Scheduler interface {
	// executionID identifies one run, not the reusable child task.
	Submit(executionID string, task ScheduledTask) error
	Cancel(executionID string) bool
}
