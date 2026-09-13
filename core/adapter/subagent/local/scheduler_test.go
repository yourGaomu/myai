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
