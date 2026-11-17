package threadpool

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
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

func TestPoolShutdownContextTimeoutAndRepeatedWaiters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := New(1, 1)
		started := make(chan struct{})
		release := make(chan struct{})
		defer pool.Shutdown()
		defer close(release)
		if err := pool.Submit(func() {
			close(started)
			<-release
		}); err != nil {
			t.Fatal(err)
		}
		<-started
		queuedRan := false
		if err := pool.Submit(func() { queuedRan = true }); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := pool.ShutdownContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("blocked task must report deadline exceeded, got %v", err)
		}
		if queuedRan {
			t.Fatal("queued task ran while the worker was blocked")
		}
		if err := pool.Submit(func() {}); !errors.Is(err, asyncport.ErrExecutorClosed) {
			t.Fatalf("shutdown must stop admission, got %v", err)
		}

		canceled, cancelWait := context.WithCancel(context.Background())
		cancelWait()
		if err := pool.ShutdownContext(canceled); !errors.Is(err, context.Canceled) {
			t.Fatalf("repeated close falsely reported completion: %v", err)
		}

		const waiters = 4
		results := make(chan error, waiters)
		for range waiters {
			go func() { results <- pool.ShutdownContext(context.Background()) }()
		}
		legacyDone := make(chan struct{})
		go func() {
			pool.Shutdown()
			close(legacyDone)
		}()
		synctest.Wait()
		select {
		case err := <-results:
			t.Fatalf("repeated waiter returned before drain: %v", err)
		case <-legacyDone:
			t.Fatal("legacy shutdown returned before drain")
		default:
		}

		release <- struct{}{}
		for range waiters {
			if err := <-results; err != nil {
				t.Fatalf("drained shutdown failed: %v", err)
			}
		}
		<-legacyDone
		if !queuedRan {
			t.Fatal("shutdown did not drain the accepted queue")
		}
		if err := pool.ShutdownContext(canceled); err != nil {
			t.Fatalf("completed shutdown must remain successful, got %v", err)
		}
	})
}

func TestPoolLegacyShutdownHasBoundedWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := New(1, 1)
		release := make(chan struct{})
		defer pool.Shutdown()
		defer close(release)
		if err := pool.Submit(func() { <-release }); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		started := time.Now()
		pool.Shutdown()
		if elapsed := time.Since(started); elapsed != defaultShutdownTimeout {
			t.Fatalf("legacy shutdown waited %s, want %s", elapsed, defaultShutdownTimeout)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := pool.ShutdownContext(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("legacy timeout must not mark unfinished work complete, got %v", err)
		}
	})
}
