package generation

import (
	"errors"
	"testing"

	runtimeservice "myai/core/application/runtime/service"
	asyncport "myai/core/port/async"
)

func TestSessionQueueDrainsOneSessionInSubmissionOrder(t *testing.T) {
	executor := &queuedExecutor{}
	queue := NewSessionQueue(runtimeservice.AsyncTaskService{Executor: executor})
	var values []string

	queue.Submit("session-1", func() { values = append(values, "user") })
	queue.Submit("session-1", func() { values = append(values, "tool") })
	queue.Submit("session-1", func() { values = append(values, "assistant") })

	if len(executor.tasks) != 1 {
		t.Fatalf("expected one worker task for a session, got %d", len(executor.tasks))
	}
	executor.tasks[0]()
	if got := len(values); got != 3 {
		t.Fatalf("expected three tasks to run, got %d", got)
	}
	if values[0] != "user" || values[1] != "tool" || values[2] != "assistant" {
		t.Fatalf("unexpected task order: %#v", values)
	}
}

func TestSessionQueueSchedulesDifferentSessionsIndependently(t *testing.T) {
	executor := &queuedExecutor{}
	queue := NewSessionQueue(runtimeservice.AsyncTaskService{Executor: executor})

	queue.Submit("session-1", func() {})
	queue.Submit("session-2", func() {})

	if len(executor.tasks) != 2 {
		t.Fatalf("expected independent workers for two sessions, got %d", len(executor.tasks))
	}
}

func TestSessionQueueRejectsSubmissionAfterExecutorCloses(t *testing.T) {
	executor := &queuedExecutor{err: asyncport.ErrExecutorClosed}
	queue := NewSessionQueue(runtimeservice.AsyncTaskService{Executor: executor})
	executed := false

	err := queue.Submit("session-1", func() { executed = true })
	if !errors.Is(err, asyncport.ErrExecutorClosed) || executed {
		t.Fatalf("expected closed executor rejection: executed=%v err=%v", executed, err)
	}
	if len(queue.queues) != 0 {
		t.Fatalf("failed submission left a queued session: %#v", queue.queues)
	}
}

type queuedExecutor struct {
	tasks []func()
	err   error
}

func (e *queuedExecutor) Submit(task func()) error {
	e.tasks = append(e.tasks, task)
	return e.err
}
