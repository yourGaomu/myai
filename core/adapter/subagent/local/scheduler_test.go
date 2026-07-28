package local

import (
	"context"
	"testing"
	"time"
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
