package local

import (
	"context"
	"errors"
	"testing"
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
