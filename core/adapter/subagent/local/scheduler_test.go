package local

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	subagentport "myai/core/port/subagent"
)

func TestSchedulerContinuesAfterTaskPanic(t *testing.T) {
	scheduler := NewScheduler(1, 2)
	defer scheduler.Close()
	if err := scheduler.Submit("panic-task", func(context.Context) {
		panic("test panic")
	}); err != nil {
		t.Fatal(err)
	}
	completed := make(chan struct{})
	if err := scheduler.Submit("next-task", func(context.Context) {
		close(completed)
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-completed:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler worker stopped after task panic")
	}
}

func TestSchedulerReportsQueueFullWithSentinelError(t *testing.T) {
	scheduler := NewScheduler(1, 1)
	defer scheduler.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	if err := scheduler.Submit("running", func(context.Context) {
		close(started)
		<-release
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first scheduler task did not start")
	}
	if err := scheduler.Submit("queued", func(context.Context) {}); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Submit("overflow", func(context.Context) {}); !errors.Is(err, subagentport.ErrSchedulerQueueFull) {
		t.Fatalf("expected queue-full sentinel, got %v", err)
	}
	close(release)
}

func TestSchedulerParkReleasesSlotForQueuedDescendant(t *testing.T) {
	scheduler := NewScheduler(1, 2)
	defer scheduler.Close()
	parentParked := make(chan struct{})
	childStarted := make(chan struct{})
	childDone := make(chan struct{})
	parentDone := make(chan struct{})
	if err := scheduler.Submit("parent", func(context.Context) {
		if err := scheduler.Park("parent"); err != nil {
			t.Errorf("park parent: %v", err)
			return
		}
		close(parentParked)
		defer func() {
			if err := scheduler.Unpark("parent"); err != nil {
				t.Errorf("unpark parent: %v", err)
			}
			close(parentDone)
		}()
		<-childDone
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-parentParked:
	case <-time.After(2 * time.Second):
		t.Fatal("parent did not park")
	}
	if err := scheduler.Submit("child", func(context.Context) {
		close(childStarted)
		close(childDone)
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-childStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("parked parent kept the only worker; descendant never started")
	}
	select {
	case <-parentDone:
	case <-time.After(2 * time.Second):
		t.Fatal("parent did not resume after descendant finished")
	}
}

func TestSchedulerCloseContextCancelsRunningAndQueuedTasks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		scheduler := NewScheduler(1, 1)
		started := make(chan struct{})
		canceled := make(chan struct{})
		release := make(chan struct{})
		defer scheduler.Close()
		defer close(release)
		if err := scheduler.Submit("running", func(ctx context.Context) {
			close(started)
			<-ctx.Done()
			close(canceled)
			<-release
		}); err != nil {
			t.Fatal(err)
		}
		<-started
		queuedResult := make(chan error, 1)
		if err := scheduler.Submit("queued", func(ctx context.Context) {
			queuedResult <- ctx.Err()
		}); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := scheduler.CloseContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("task still cleaning up must report a timeout, got %v", err)
		}
		<-canceled
		if err := scheduler.Submit("late", func(context.Context) {}); !errors.Is(err, subagentport.ErrSchedulerClosed) {
			t.Fatalf("close must stop admission, got %v", err)
		}
		if err := scheduler.Park("running"); !errors.Is(err, subagentport.ErrSchedulerClosed) {
			t.Fatalf("close must stop substitute workers, got %v", err)
		}
		canceledWait, cancelWait := context.WithCancel(context.Background())
		cancelWait()
		if err := scheduler.CloseContext(canceledWait); !errors.Is(err, context.Canceled) {
			t.Fatalf("repeated close falsely reported completion: %v", err)
		}

		const waiters = 4
		results := make(chan error, waiters)
		for range waiters {
			go func() { results <- scheduler.CloseContext(context.Background()) }()
		}
		legacyDone := make(chan struct{})
		go func() {
			scheduler.Close()
			close(legacyDone)
		}()
		synctest.Wait()
		select {
		case err := <-results:
			t.Fatalf("repeated close returned before workers exited: %v", err)
		case <-legacyDone:
			t.Fatal("legacy close returned before workers exited")
		default:
		}

		release <- struct{}{}
		for range waiters {
			if err := <-results; err != nil {
				t.Fatalf("completed shutdown failed: %v", err)
			}
		}
		<-legacyDone
		if err := <-queuedResult; !errors.Is(err, context.Canceled) {
			t.Fatalf("queued cleanup must receive a canceled context, got %v", err)
		}
		if len(scheduler.active) != 0 {
			t.Fatal("close did not remove all accepted tasks")
		}
		if err := scheduler.CloseContext(canceledWait); err != nil {
			t.Fatalf("completed close must remain successful, got %v", err)
		}
	})
}

func TestSchedulerCloseContextWaitsForParkSubstituteWorker(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		scheduler := NewScheduler(1, 1)
		defer scheduler.Close()
		parked := make(chan struct{})
		parentDone := make(chan struct{})
		if err := scheduler.Submit("parent", func(ctx context.Context) {
			if err := scheduler.Park("parent"); err != nil {
				t.Errorf("park parent: %v", err)
				return
			}
			close(parked)
			<-ctx.Done()
			if err := scheduler.Unpark("parent"); err != nil {
				t.Errorf("unpark parent: %v", err)
			}
			close(parentDone)
		}); err != nil {
			t.Fatal(err)
		}
		<-parked
		started := make(chan struct{})
		canceled := make(chan struct{})
		release := make(chan struct{})
		defer close(release)
		if err := scheduler.Submit("child", func(ctx context.Context) {
			close(started)
			<-ctx.Done()
			close(canceled)
			<-release
		}); err != nil {
			t.Fatal(err)
		}
		<-started

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := scheduler.CloseContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("running substitute must prevent completed close, got %v", err)
		}
		<-parentDone
		<-canceled
		result := make(chan error, 1)
		go func() { result <- scheduler.CloseContext(context.Background()) }()
		synctest.Wait()
		select {
		case err := <-result:
			t.Fatalf("close did not wait for substitute cleanup: %v", err)
		default:
		}
		release <- struct{}{}
		if err := <-result; err != nil {
			t.Fatalf("substitute did not join: %v", err)
		}
	})
}

func TestSchedulerCancelReachesTaskContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		scheduler := NewScheduler(1, 1)
		defer scheduler.Close()
		started := make(chan struct{})
		result := make(chan error, 1)
		if err := scheduler.Submit("running", func(ctx context.Context) {
			close(started)
			<-ctx.Done()
			result <- ctx.Err()
		}); err != nil {
			t.Fatal(err)
		}
		<-started
		if !scheduler.Cancel("running") {
			t.Fatal("cancel did not find the running task")
		}
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("task did not observe cancellation: %v", err)
		}
		synctest.Wait()
		if scheduler.Cancel("running") {
			t.Fatal("completed task remained active")
		}
	})
}

func TestSchedulerLegacyCloseHasBoundedWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		scheduler := NewScheduler(1, 1)
		release := make(chan struct{})
		defer scheduler.Close()
		defer close(release)
		if err := scheduler.Submit("uncooperative", func(context.Context) { <-release }); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		started := time.Now()
		scheduler.Close()
		if elapsed := time.Since(started); elapsed != defaultCloseTimeout {
			t.Fatalf("legacy close waited %s, want %s", elapsed, defaultCloseTimeout)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := scheduler.CloseContext(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("legacy timeout must leave shutdown incomplete, got %v", err)
		}
	})
}
