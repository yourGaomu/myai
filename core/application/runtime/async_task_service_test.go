package runtime

import (
	"errors"
	"testing"
)

func TestAsyncTaskServiceUsesExecutor(t *testing.T) {
	executor := &fakeAsyncExecutor{}
	fallbackCalled := false

	(AsyncTaskService{
		Executor: executor,
		Fallback: func(func()) { fallbackCalled = true },
	}).Submit(func() {})

	if executor.task == nil || fallbackCalled {
		t.Fatalf("expected executor submission without fallback: executor=%#v fallback=%v", executor, fallbackCalled)
	}
}

func TestAsyncTaskServiceFallsBackWhenSubmissionFails(t *testing.T) {
	executor := &fakeAsyncExecutor{err: errors.New("queue full")}
	fallbackCalled := false
	taskCalled := false

	(AsyncTaskService{
		Executor: executor,
		Fallback: func(task func()) {
			fallbackCalled = true
			task()
		},
	}).Submit(func() { taskCalled = true })

	if !fallbackCalled || !taskCalled {
		t.Fatalf("expected fallback task execution: fallback=%v task=%v", fallbackCalled, taskCalled)
	}
}

func TestAsyncTaskServiceIgnoresNilTask(t *testing.T) {
	executor := &fakeAsyncExecutor{}
	(AsyncTaskService{Executor: executor}).Submit(nil)
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
