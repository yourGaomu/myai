package mapper

import (
	"testing"
	"time"

	"myai/core/adapter/persistence/mongo/subagent/po"
	domainsubagent "myai/core/domain/subagent"
	domainworkspace "myai/core/domain/workspace"
	modelport "myai/core/port/model"
)

func TestTaskWorkspaceSourceRootRoundTrips(t *testing.T) {
	original := domainsubagent.Task{ID: "task-1", Workspace: domainworkspace.Reference{
		ID: "workspace-1", Mode: domainworkspace.IsolationModeSnapshot, Root: "/tmp/snapshot", SourceRoot: "/src/project",
	}}
	roundTrip := TaskDomainFromDocument(TaskDocumentFromDomain(original))
	if roundTrip.Workspace.SourceRoot != original.Workspace.SourceRoot || roundTrip.Workspace.Root != original.Workspace.Root {
		t.Fatalf("workspace source identity did not round trip: %#v", roundTrip.Workspace)
	}
}

func TestTaskMailboxRoundTrips(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	claimedAt := now.Add(time.Second)
	original := domainsubagent.Task{ID: "task-1", Mailbox: []domainsubagent.Message{{
		ID: "message-1", Content: "follow up", Trigger: domainsubagent.AgentMessageTriggerTurn, Status: domainsubagent.MessageStatusDelivering,
		DeliveryAttempts: 2, LastError: "previous failure", CreatedAt: now, ClaimedAt: &claimedAt,
	}}}
	roundTrip := TaskDomainFromDocument(TaskDocumentFromDomain(original))
	if len(roundTrip.Mailbox) != 1 || roundTrip.Mailbox[0].ID != "message-1" || roundTrip.Mailbox[0].Content != "follow up" || roundTrip.Mailbox[0].Trigger != domainsubagent.AgentMessageTriggerTurn ||
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
	original := domainsubagent.Run{ID: "run-2", TaskID: "task-1", RequestID: "followup-request", RequestContentHash: "original-input-hash", Instruction: "continue",
		Usage: modelport.TokenUsage{PromptTokens: 12, CompletionTokens: 8, TotalTokens: 20, ReasoningTokens: 3, PromptCachedTokens: 4, Available: true}}
	roundTrip := RunDomainFromDocument(RunDocumentFromDomain(original))
	if roundTrip.RequestID != original.RequestID || roundTrip.RequestContentHash != original.RequestContentHash || roundTrip.Instruction != original.Instruction || roundTrip.Usage != original.Usage {
		t.Fatalf("follow-up identity did not survive persistence: %#v", roundTrip)
	}
}

func TestTaskUsageRoundTrips(t *testing.T) {
	original := domainsubagent.Task{ID: "task-1", Usage: modelport.TokenUsage{PromptTokens: 21, CompletionTokens: 9, TotalTokens: 30, Available: true}}
	roundTrip := TaskDomainFromDocument(TaskDocumentFromDomain(original))
	if roundTrip.Usage != original.Usage {
		t.Fatalf("task usage did not survive persistence: %#v", roundTrip.Usage)
	}
}
