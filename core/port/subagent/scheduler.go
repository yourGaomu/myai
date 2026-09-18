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
	// Park releases a running slot so queued descendants can run while this
	// execution blocks in wait_agent. Unpark reacquires a slot before the
	// parent continues. Both are refcounted per execution.
	Park(executionID string) error
	Unpark(executionID string) error
}
