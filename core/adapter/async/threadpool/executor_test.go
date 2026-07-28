package threadpool

import (
	"errors"
	"testing"
	"time"

	asyncport "myai/core/port/async"
)

func TestExecutorSubmitsTaskToThreadPool(t *testing.T) {
	pool := New(1, 1)
	defer pool.Shutdown()
	executed := make(chan struct{}, 1)

	if err := (Executor{Pool: pool}).Submit(func() { executed <- struct{}{} }); err != nil {
		t.Fatal(err)
	}

	select {
	case <-executed:
	case <-time.After(time.Second):
		t.Fatal("expected submitted task to execute")
	}
}

func TestExecutorRejectsNilPool(t *testing.T) {
	if err := (Executor{}).Submit(func() {}); err == nil {
		t.Fatal("expected nil thread pool error")
	}
}

func TestPoolWorkerContinuesAfterTaskPanic(t *testing.T) {
	pool := New(1, 2)
	defer pool.Shutdown()
	executed := make(chan struct{}, 1)

	if err := pool.Submit(func() { panic("background failure") }); err != nil {
		t.Fatal(err)
	}
	if err := pool.Submit(func() { executed <- struct{}{} }); err != nil {
		t.Fatal(err)
	}

	select {
	case <-executed:
	case <-time.After(time.Second):
		t.Fatal("worker stopped after a task panic")
	}
}

func TestPoolReportsQueueFullAndClosedWithStableErrors(t *testing.T) {
	pool := New(1, 1)
	started := make(chan struct{})
	release := make(chan struct{})
	if err := pool.Submit(func() {
		close(started)
		<-release
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := pool.Submit(func() {}); err != nil {
		t.Fatal(err)
	}
	if err := pool.Submit(func() {}); !errors.Is(err, asyncport.ErrQueueFull) {
		t.Fatalf("expected queue full error, got %v", err)
	}
	close(release)
	pool.Shutdown()
	if err := pool.Submit(func() {}); !errors.Is(err, asyncport.ErrExecutorClosed) {
		t.Fatalf("expected closed executor error, got %v", err)
	}
}
