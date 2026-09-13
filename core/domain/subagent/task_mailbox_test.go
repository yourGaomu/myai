package subagent

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestMailboxClaimReleaseAndAcknowledge(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	task := Task{ID: "task-1", Status: TaskStatusRunning}
	if err := task.EnqueueMessage(Message{ID: "message-1", Content: "follow up", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	claimed, ok := task.ClaimMailboxMessage(now.Add(time.Second))
	if !ok || claimed.Status != MessageStatusDelivering || claimed.DeliveryAttempts != 1 {
		t.Fatalf("message was not claimed: %#v", claimed)
	}
	deliveryErr := errors.New("model unavailable")
	if !task.ReleaseMailboxMessage(claimed.ID, deliveryErr, now.Add(2*time.Second)) {
		t.Fatal("expected claimed message to be released")
	}
	if task.Mailbox[0].Status != MessageStatusPending || task.Mailbox[0].LastError != deliveryErr.Error() {
		t.Fatalf("released message was not retryable: %#v", task.Mailbox[0])
	}
	claimed, ok = task.ClaimMailboxMessage(now.Add(3 * time.Second))
	if !ok || claimed.DeliveryAttempts != 2 {
		t.Fatalf("released message was not reclaimed: %#v", claimed)
	}
	if !task.AcknowledgeMailboxMessage(claimed.ID, now.Add(4*time.Second)) || len(task.Mailbox) != 0 {
		t.Fatalf("acknowledged message was not removed: %#v", task.Mailbox)
	}
}

func TestMailboxRejectsUnsupportedStateAndCapacityOverflow(t *testing.T) {
	waiting := Task{ID: "task-waiting", Status: TaskStatusWaitingSubagents}
	if err := waiting.EnqueueMessage(Message{ID: "message-1", Content: "continue"}); err != nil {
		t.Fatalf("waiting_subagents task should accept a wake-up message: %v", err)
	}
	permissionWaiting := Task{ID: "task-permission", Status: TaskStatusWaitingPermission}
	if err := permissionWaiting.EnqueueMessage(Message{ID: "message-2", Content: "continue"}); err == nil {
		t.Fatal("expected waiting_permission task mailbox to reject a message")
	}

	running := Task{ID: "task-running", Status: TaskStatusRunning}
	chunk := strings.Repeat("a", MaxMailboxMessageRunes)
	for index := 0; index < MaxMailboxRunes/MaxMailboxMessageRunes; index++ {
		if err := running.EnqueueMessage(Message{ID: string(rune('a' + index)), Content: chunk}); err != nil {
			t.Fatalf("fill mailbox: %v", err)
		}
	}
	if err := running.EnqueueMessage(Message{ID: "overflow", Content: "x"}); err == nil {
		t.Fatal("expected aggregate mailbox capacity error")
	}
	if err := running.MarkSucceeded("done", "", time.Now()); err == nil {
		t.Fatal("expected completion to reject pending mailbox messages")
	}
}

func TestMailboxAcceptsExactlyMaximumMessageCount(t *testing.T) {
	task := Task{ID: "task-running", Status: TaskStatusRunning}
	for index := 0; index < MaxMailboxMessages; index++ {
		if err := task.EnqueueMessage(Message{ID: fmt.Sprintf("message-%d", index), Content: "x"}); err != nil {
			t.Fatalf("message %d should fit: %v", index+1, err)
		}
	}
	if len(task.Mailbox) != MaxMailboxMessages {
		t.Fatalf("expected %d mailbox messages, got %d", MaxMailboxMessages, len(task.Mailbox))
	}
	if err := task.EnqueueMessage(Message{ID: "overflow", Content: "x"}); err == nil {
		t.Fatal("expected the message after the capacity boundary to fail")
	}
}
