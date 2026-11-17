package generation

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestSessionQueueDrainsAcceptedSubmissionAfterExecutorCloses(t *testing.T) {
	executor := &queuedExecutor{err: asyncport.ErrExecutorClosed}
	queue := NewSessionQueue(runtimeservice.AsyncTaskService{Executor: executor})
	executed := false

	err := queue.Submit("session-1", func() { executed = true })
	if err != nil || !executed {
		t.Fatalf("accepted task was not drained: executed=%v err=%v", executed, err)
	}
	if len(queue.queues) != 0 {
		t.Fatalf("failed submission left a queued session: %#v", queue.queues)
	}
}

func TestSessionQueueSubmitAndWaitIgnoresCancellationAfterExecutionStarts(t *testing.T) {
	executor := &signalingExecutor{tasks: make(chan func(), 1)}
	queue := NewSessionQueue(runtimeservice.AsyncTaskService{Executor: executor})
	ctx, cancel := context.WithCancel(context.Background())
	completed := make(chan error, 1)
	started, release := make(chan struct{}), make(chan struct{})
	go func() {
		completed <- queue.SubmitAndWait(ctx, "session-1", func() error { close(started); <-release; return nil })
	}()

	scheduled := <-executor.tasks
	go scheduled()
	<-started
	cancel()
	select {
	case err := <-completed:
		t.Fatalf("wait returned before started persistence completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-completed; err != nil {
		t.Fatalf("unexpected persistence result: %v", err)
	}
}

func TestSessionQueueSubmitAndWaitRejectsAlreadyCanceledContext(t *testing.T) {
	executor := &signalingExecutor{tasks: make(chan func(), 1)}
	queue := NewSessionQueue(runtimeservice.AsyncTaskService{Executor: executor})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := queue.SubmitAndWait(ctx, "session-1", func() error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled submission, got %v", err)
	}
	if len(executor.tasks) != 0 {
		t.Fatal("canceled context scheduled persistence")
	}
}

func TestSessionQueueFailedHeadReturnsWaitingMutationAndNeverRetriesIt(t *testing.T) {
	executor := &signalingExecutor{tasks: make(chan func(), 2)}
	queue := NewSessionQueue(runtimeservice.AsyncTaskService{Executor: executor})
	failure := errors.New("storage unavailable")
	fail := true
	if err := queue.SubmitTask("session", func() error {
		if fail {
			return failure
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	worker := <-executor.tasks
	mutations := 0
	result := make(chan error, 1)
	go func() {
		result <- queue.SubmitAndWait(context.Background(), "session", func() error { mutations++; return nil })
	}()
	deadline := time.After(time.Second)
	for {
		queue.mu.Lock()
		count := len(queue.queues["session"].tasks)
		queue.mu.Unlock()
		if count == 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("waiter not queued")
		default:
		}
	}
	worker()
	select {
	case err := <-result:
		if !errors.Is(err, failure) {
			t.Fatalf("waiter error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("head failure blocked synchronous mutation")
	}
	fail = false
	flushed := make(chan error, 1)
	go func() { flushed <- queue.Flush(context.Background()) }()
	(<-executor.tasks)()
	if err := <-flushed; err != nil {
		t.Fatal(err)
	}
	if mutations != 0 {
		t.Fatal("rolled-back mutation executed on retry")
	}
}

func TestSessionQueueRetainsFailedWriteForFlushRetryInOrder(t *testing.T) {
	queue := NewSessionQueue(runtimeservice.AsyncTaskService{})
	failure := errors.New("offline")
	fail := true
	var values []string
	queue.SubmitTask("session", func() error {
		if fail {
			return failure
		}
		values = append(values, "user")
		return nil
	})
	queue.SubmitTask("session", func() error { values = append(values, "assistant"); return nil })
	if err := queue.Flush(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("missing pending error %v", err)
	}
	if len(values) != 0 {
		t.Fatal("failed head lost ordering")
	}
	fail = false
	if err := queue.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0] != "user" || values[1] != "assistant" {
		t.Fatalf("retry order=%v", values)
	}
}

func TestSessionQueuePanicDoesNotExposePayload(t *testing.T) {
	queue := NewSessionQueue(runtimeservice.AsyncTaskService{})
	var reported error
	queue.OnPanic = func(err error) { reported = err }
	if err := queue.SubmitTask("session", func() error { panic("private payload sentinel") }); err != nil {
		t.Fatal(err)
	}
	if reported == nil || reported.Error() != "session persistence task panicked" {
		t.Fatalf("panic error=%v", reported)
	}
	if err := queue.Flush(context.Background()); err == nil {
		t.Fatal("panic lost pending work")
	}
}

func TestSessionQueueCancelsQueuedMutationBeforeExecution(t *testing.T) {
	executor := &signalingExecutor{tasks: make(chan func(), 1)}
	queue := NewSessionQueue(runtimeservice.AsyncTaskService{Executor: executor})
	queue.Submit("session", func() {})
	worker := <-executor.tasks
	ctx, cancel := context.WithCancel(context.Background())
	executed := false
	result := make(chan error, 1)
	go func() { result <- queue.SubmitAndWait(ctx, "session", func() error { executed = true; return nil }) }()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel blocked")
	}
	worker()
	if executed {
		t.Fatal("canceled queued mutation executed")
	}
}

type queuedExecutor struct {
	tasks []func()
	err   error
}

type signalingExecutor struct {
	tasks chan func()
}

func (e *signalingExecutor) Submit(task func()) error {
	e.tasks <- task
	return nil
}

func (e *queuedExecutor) Submit(task func()) error {
	e.tasks = append(e.tasks, task)
	return e.err
}
