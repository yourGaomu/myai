package pendinginput

import (
	"strings"
	"testing"
	"time"

	domaingeneration "myai/core/domain/generation"
	domainsubagent "myai/core/domain/subagent"
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

func TestQueueRequeuesIdentifiedBatchWithoutDuplicatingIDs(t *testing.T) {
	queue := NewQueue()
	if err := queue.EnqueueIdentified("session-1", "message-1", "result"); err != nil {
		t.Fatal(err)
	}
	claimed := queue.DrainIdentified("session-1")
	if err := queue.EnqueueIdentified("session-1", "message-2", "later"); err != nil {
		t.Fatal(err)
	}
	if err := queue.RequeueIdentified("session-1", append(claimed, domaingeneration.PendingTurnInputItem{ID: "message-1", Content: "result"})); err != nil {
		t.Fatal(err)
	}
	items := queue.DrainIdentified("session-1")
	if len(items) != 2 || items[0].ID != "message-1" || items[1].ID != "message-2" {
		t.Fatalf("unexpected requeued items: %#v", items)
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

type recordingAcknowledger struct {
	ids []string
}

func (acknowledger *recordingAcknowledger) Acknowledge(messageID string) error {
	acknowledger.ids = append(acknowledger.ids, messageID)
	return nil
}

func TestQueueIdentifiedMessagesAreIdempotentAndAcknowledged(t *testing.T) {
	queue := NewQueue()
	acknowledger := &recordingAcknowledger{}
	queue.SetAcknowledger(acknowledger)
	if err := queue.EnqueueIdentified("session-1", "message-1", "result"); err != nil {
		t.Fatal(err)
	}
	if err := queue.EnqueueIdentified("session-1", "message-1", "result"); err != nil {
		t.Fatal(err)
	}
	items := queue.DrainIdentified("session-1")
	if len(items) != 1 || items[0].ID != "message-1" || items[0].Content != "result" {
		t.Fatalf("unexpected identified items: %#v", items)
	}
	if err := queue.Acknowledge("message-1"); err != nil {
		t.Fatal(err)
	}
	if len(acknowledger.ids) != 1 || acknowledger.ids[0] != "message-1" {
		t.Fatalf("unexpected acknowledgements: %#v", acknowledger.ids)
	}
}

func TestQueueRejectsConflictingReuseOfMessageID(t *testing.T) {
	queue := NewQueue()
	if err := queue.EnqueueIdentified("session-1", "message-1", "first"); err != nil {
		t.Fatal(err)
	}
	if err := queue.EnqueueIdentified("session-1", "message-1", "second"); err == nil {
		t.Fatal("expected conflicting queued message to fail")
	}
	claimed := queue.DrainIdentified("session-1")
	if err := queue.EnqueueIdentified("session-1", "message-1", "first"); err != nil {
		t.Fatal(err)
	}
	if err := queue.EnqueueIdentified("session-1", "message-2", "first"); err != nil {
		t.Fatal(err)
	}
	if err := queue.EnqueueIdentified("session-1", "message-1", "second"); err == nil {
		t.Fatal("expected conflicting in-flight message to fail")
	}
	if err := queue.RequeueIdentified("session-1", claimed); err != nil {
		t.Fatal(err)
	}
}

func TestQueueAllowsSameContentForDifferentMessageIDs(t *testing.T) {
	queue := NewQueue()
	if err := queue.EnqueueIdentified("session-1", "message-1", "same"); err != nil {
		t.Fatal(err)
	}
	if err := queue.EnqueueIdentified("session-1", "message-2", "same"); err != nil {
		t.Fatal(err)
	}
	items := queue.DrainIdentified("session-1")
	if len(items) != 2 || items[0].ID != "message-1" || items[1].ID != "message-2" {
		t.Fatalf("same content from different events was collapsed: %#v", items)
	}
}

func TestQueueTreatsSameMessageWithDifferentCreatedAtAsIdempotent(t *testing.T) {
	queue := NewQueue()
	first := domainsubagent.AgentMessage{
		ID: "message-time", RecipientAgentID: "parent-1", Kind: domainsubagent.AgentMessageKindTaskResult,
		Content: "same result", Trigger: domainsubagent.AgentMessageTriggerTurn,
		Status: domainsubagent.AgentMessagePending, CreatedAt: time.Now().UTC(),
	}
	if err := queue.EnqueueAgentMessage("parent-1", first); err != nil {
		t.Fatal(err)
	}
	second := domainsubagent.CloneAgentMessage(first)
	second.CreatedAt = second.CreatedAt.Add(time.Minute)
	if err := queue.EnqueueAgentMessage("parent-1", second); err != nil {
		t.Fatalf("same message ID with a reconstructed timestamp should be idempotent: %v", err)
	}
	items := queue.DrainIdentified("parent-1")
	if len(items) != 1 || items[0].ID != first.ID {
		t.Fatalf("expected one queued message, got %#v", items)
	}
}
