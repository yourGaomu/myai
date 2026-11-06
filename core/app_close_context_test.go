package core

import (
	"context"
	"errors"
	"testing"
)

type shutdownTestCloser struct {
	close func() error
}

func (c shutdownTestCloser) Close() error { return c.close() }

func TestApplicationCloseContextHonorsRecoveryCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	recoveryStopped := false
	dependencyClosed := false
	app := &Application{
		memoryRecoveryCancel: func() { recoveryStopped = true },
		memoryRecoveryDone:   make(chan struct{}),
		workspaceCloser: shutdownTestCloser{close: func() error {
			dependencyClosed = true
			return nil
		}},
	}
	if err := app.CloseContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation while waiting for producer, got %v", err)
	}
	if !recoveryStopped {
		t.Fatal("recovery producer was not canceled")
	}
	if dependencyClosed {
		t.Fatal("dependencies must remain open when producers have not stopped")
	}
}

func TestApplicationCloseContextClosesResourcesAndReportsFailure(t *testing.T) {
	want := errors.New("close failure")
	closed := make(chan struct{}, 1)
	app := &Application{workspaceCloser: shutdownTestCloser{close: func() error {
		closed <- struct{}{}
		return want
	}}}
	if err := app.CloseContext(context.Background()); !errors.Is(err, want) {
		t.Fatalf("expected resource close error, got %v", err)
	}
	select {
	case <-closed:
	default:
		t.Fatal("workspace resource was not closed")
	}
}

func TestNilApplicationCloseContext(t *testing.T) {
	var app *Application
	if err := app.CloseContext(context.Background()); err != nil {
		t.Fatal(err)
	}
}
