package mapper

import (
	"testing"
	"time"

	"myai/core/adapter/persistence/mongo/subagent/po"
	domainsubagent "myai/core/domain/subagent"
)

func TestTaskMailboxRoundTrips(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	claimedAt := now.Add(time.Second)
	original := domainsubagent.Task{ID: "task-1", Mailbox: []domainsubagent.Message{{
		ID: "message-1", Content: "follow up", Status: domainsubagent.MessageStatusDelivering,
		DeliveryAttempts: 2, LastError: "previous failure", CreatedAt: now, ClaimedAt: &claimedAt,
	}}}
	roundTrip := TaskDomainFromDocument(TaskDocumentFromDomain(original))
	if len(roundTrip.Mailbox) != 1 || roundTrip.Mailbox[0].ID != "message-1" || roundTrip.Mailbox[0].Content != "follow up" ||
		roundTrip.Mailbox[0].Status != domainsubagent.MessageStatusDelivering || roundTrip.Mailbox[0].DeliveryAttempts != 2 ||
		roundTrip.Mailbox[0].ClaimedAt == nil || !roundTrip.Mailbox[0].ClaimedAt.Equal(claimedAt) {
		t.Fatalf("mailbox did not round trip: %#v", roundTrip.Mailbox)
	}
}

func TestTaskDomainFromDocumentClearsLegacyCanceledUnreadFlag(t *testing.T) {
	task := TaskDomainFromDocument(po.TaskDocument{
		ID: "task-1", Status: string(domainsubagent.TaskStatusCanceled), Unread: true,
	})
	if task.Unread {
		t.Fatalf("legacy canceled task must load as consumed: %#v", task)
	}
}

func TestRunFollowupRequestIDRoundTrips(t *testing.T) {
	original := domainsubagent.Run{ID: "run-2", TaskID: "task-1", RequestID: "followup-request", RequestContentHash: "original-input-hash", Instruction: "continue"}
	roundTrip := RunDomainFromDocument(RunDocumentFromDomain(original))
	if roundTrip.RequestID != original.RequestID || roundTrip.RequestContentHash != original.RequestContentHash || roundTrip.Instruction != original.Instruction {
		t.Fatalf("follow-up identity did not survive persistence: %#v", roundTrip)
	}
}
