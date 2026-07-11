package threadpool

import (
	"testing"
	"time"
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
