package runtime

import (
	"errors"
	"testing"

	asyncport "myai/core/port/async"
)

func TestAsyncTaskServiceUsesExecutor(t *testing.T) {
	executor := &fakeAsyncExecutor{}

	err := (AsyncTaskService{Executor: executor}).Submit(func() {})

	if err != nil || executor.task == nil {
		t.Fatalf("expected executor submission: executor=%#v err=%v", executor, err)
	}
}

func TestAsyncTaskServiceRunsInCallerWhenQueueIsFull(t *testing.T) {
	executor := &fakeAsyncExecutor{err: asyncport.ErrQueueFull}
	taskCalled := false

	err := (AsyncTaskService{Executor: executor}).Submit(func() { taskCalled = true })

	if err != nil || !taskCalled {
		t.Fatalf("expected caller-runs task execution: task=%v err=%v", taskCalled, err)
	}
}

func TestAsyncTaskServiceDoesNotRunAfterExecutorClosed(t *testing.T) {
	executor := &fakeAsyncExecutor{err: asyncport.ErrExecutorClosed}
	taskCalled := false
	err := (AsyncTaskService{Executor: executor}).Submit(func() { taskCalled = true })
	if !errors.Is(err, asyncport.ErrExecutorClosed) || taskCalled {
		t.Fatalf("expected closed executor rejection without execution: task=%v err=%v", taskCalled, err)
	}
}

func TestAsyncTaskServiceRejectsMissingExecutor(t *testing.T) {
	err := (AsyncTaskService{}).Submit(func() {})
	if !errors.Is(err, asyncport.ErrExecutorUnavailable) {
		t.Fatalf("expected unavailable executor error, got %v", err)
	}
}

func TestAsyncTaskServiceIgnoresNilTask(t *testing.T) {
	executor := &fakeAsyncExecutor{}
	if err := (AsyncTaskService{Executor: executor}).Submit(nil); err != nil {
		t.Fatal(err)
	}
	if executor.task != nil {
		t.Fatal("expected nil task to be ignored")
	}
}

type fakeAsyncExecutor struct {
	task func()
	err  error
}

func (e *fakeAsyncExecutor) Submit(task func()) error {
	e.task = task
	return e.err
}
