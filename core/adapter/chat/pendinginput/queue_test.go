package pendinginput

import (
	"strings"
	"testing"
)

func TestQueueEnqueueDrainAndHasPending(t *testing.T) {
	queue := NewQueue()
	if queue.HasPending("session-1") {
		t.Fatal("expected empty queue")
	}
	if err := queue.Enqueue("session-1", " first "); err != nil {
		t.Fatal(err)
	}
	if err := queue.Enqueue("session-1", "second"); err != nil {
		t.Fatal(err)
	}
	if !queue.HasPending("session-1") || queue.HasPending("session-2") {
		t.Fatal("expected pending only for session-1")
	}
	got := queue.Drain("session-1")
	if len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Fatalf("unexpected drain: %#v", got)
	}
	if queue.HasPending("session-1") || queue.Drain("session-1") != nil {
		t.Fatal("expected queue to be empty after drain")
	}
}

func TestQueueRejectsEmptyFullAndTooLong(t *testing.T) {
	queue := NewQueue()
	if err := queue.Enqueue("", "hi"); err == nil {
		t.Fatal("expected empty session id to fail")
	}
	if err := queue.Enqueue("session-1", "  "); err == nil {
		t.Fatal("expected empty content to fail")
	}
	if err := queue.Enqueue("session-1", strings.Repeat("a", MaxPendingRunes+1)); err == nil {
		t.Fatal("expected oversized content to fail")
	}
	for i := 0; i < MaxPendingMessages; i++ {
		if err := queue.Enqueue("session-1", "msg"); err != nil {
			t.Fatal(err)
		}
	}
	if err := queue.Enqueue("session-1", "overflow"); err == nil {
		t.Fatal("expected full queue to fail")
	}
}
