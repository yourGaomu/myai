package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSessionOperationCoordinatorSerializesSameSession(t *testing.T) {
	coordinator := newSessionOperationCoordinator()
	firstUnlock, err := coordinator.lock(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan func(), 1)
	go func() {
		unlock, lockErr := coordinator.lock(context.Background(), "session-1")
		if lockErr == nil {
			acquired <- unlock
		}
	}()

	select {
	case <-acquired:
		t.Fatal("second operation acquired the same session too early")
	case <-time.After(50 * time.Millisecond):
	}
	firstUnlock()
	select {
	case unlock := <-acquired:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("second operation did not acquire the released session")
	}
}

func TestSessionOperationCoordinatorAllowsDifferentSessionsAndCancellation(t *testing.T) {
	coordinator := newSessionOperationCoordinator()
	unlock, err := coordinator.lock(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	otherUnlock, err := coordinator.lock(context.Background(), "session-2")
	if err != nil {
		t.Fatal(err)
	}
	otherUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := coordinator.lock(ctx, "session-1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected canceled wait, got %v", err)
	}
}
