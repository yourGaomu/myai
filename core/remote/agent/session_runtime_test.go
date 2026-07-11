package agent

import (
	"context"
	"testing"
)

func TestSessionRuntimeManagerReusesRuntimeBySession(t *testing.T) {
	manager := newSessionRuntimeManager()
	if manager.get("session-1") != manager.get("session-1") {
		t.Fatal("expected the same runtime for one session")
	}
	if manager.get("session-1") == manager.get("session-2") {
		t.Fatal("expected different runtimes for different sessions")
	}
	if manager.get("") != manager.get("") {
		t.Fatal("expected empty session id to use stable default runtime")
	}
}

func TestSessionRuntimeRejectsConcurrentStartAndSupportsPause(t *testing.T) {
	runtime := &sessionRuntime{}
	runCtx, cancel, ok := runtime.start(context.Background())
	if !ok {
		t.Fatal("expected first start to succeed")
	}
	_, secondCancel, secondOK := runtime.start(context.Background())
	defer secondCancel()
	if secondOK {
		t.Fatal("expected concurrent start to be rejected")
	}
	if !runtime.pause() {
		t.Fatal("expected running task to pause")
	}
	<-runCtx.Done()
	runtime.finish(cancel)

	_, nextCancel, nextOK := runtime.start(context.Background())
	if !nextOK {
		t.Fatal("expected start after finish to succeed")
	}
	runtime.finish(nextCancel)
}
