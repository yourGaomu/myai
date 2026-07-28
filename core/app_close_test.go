package core

import (
	"context"
	"testing"
	"time"

	adapterthreadpool "myai/core/adapter/async/threadpool"
	subagentlocal "myai/core/adapter/subagent/local"
)

func TestApplicationCloseStopsSubagentsBeforeClosingWorkspaces(t *testing.T) {
	scheduler := subagentlocal.NewScheduler(1, 1)
	jobStopped := make(chan struct{})
	if err := scheduler.Submit("task-1", func(ctx context.Context) {
		<-ctx.Done()
		close(jobStopped)
	}); err != nil {
		t.Fatal(err)
	}
	workspace := &orderingWorkspaceCloser{jobStopped: jobStopped}
	app := &Application{subagentScheduler: scheduler, workspaceCloser: workspace}

	closed := make(chan error, 1)
	go func() {
		closed <- app.Close()
	}()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("application close timed out")
	}
	if !workspace.closedAfterJob {
		t.Fatal("expected workspace resources to close after subagent jobs stopped")
	}
}

func TestApplicationCloseDrainsThreadPoolBeforeDependencies(t *testing.T) {
	pool := adapterthreadpool.New(1, 1)
	taskDone := make(chan struct{})
	if err := pool.Submit(func() {
		time.Sleep(25 * time.Millisecond)
		close(taskDone)
	}); err != nil {
		t.Fatal(err)
	}
	dependency := &orderingDependencyCloser{taskDone: taskDone}
	app := &Application{threadPool: pool, documentProcessorCloser: dependency}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
	if !dependency.closedAfterTask {
		t.Fatal("expected background tasks to drain before dependencies close")
	}
}

type orderingWorkspaceCloser struct {
	jobStopped     <-chan struct{}
	closedAfterJob bool
}

type orderingDependencyCloser struct {
	taskDone        <-chan struct{}
	closedAfterTask bool
}

func (closer *orderingDependencyCloser) Close() error {
	select {
	case <-closer.taskDone:
		closer.closedAfterTask = true
	default:
	}
	return nil
}

func (closer *orderingWorkspaceCloser) Close() error {
	select {
	case <-closer.jobStopped:
		closer.closedAfterJob = true
	default:
	}
	return nil
}
