package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"myai/core/adapter/subagent/memory"
	domainsubagent "myai/core/domain/subagent"
)

type recordingParentNotifier struct {
	tasks []domainsubagent.AgentMessage
}

func TestParentCompletionSeparatesRunsAndReplaysSavedContent(t *testing.T) {
	ctx := context.Background()
	repository := memory.NewRepository()
	notifier := &recordingParentNotifier{}
	svc := &Service{Tasks: repository, AgentMessages: repository, ParentNotifier: notifier}
	task := domainsubagent.Task{ID: "task", CurrentRunID: "run-1", ParentSessionID: "parent", Status: domainsubagent.TaskStatusSucceeded, Result: "first result"}
	for _, run := range []string{"run-1", "run-2"} {
		task.CurrentRunID = run
		if run == "run-2" {
			task.Result = "second result"
		}
		if err := repository.SaveTask(ctx, task); err != nil {
			t.Fatal(err)
		}
		if err := svc.ensureParentCompletionQueued(task); err != nil {
			t.Fatal(err)
		}
		if err := svc.ensureParentCompletionQueued(task); err != nil {
			t.Fatal(err)
		}
	}
	if len(notifier.tasks) != 2 || notifier.tasks[0].ID == notifier.tasks[1].ID {
		t.Fatalf("runs were deduplicated: %#v", notifier.tasks)
	}
	// Recovery must replay run-1's envelope even though the current task is run-2.
	recovered := &recordingParentNotifier{}
	restarted := &Service{Tasks: repository, AgentMessages: repository, ParentNotifier: recovered}
	if err := restarted.RecoverPendingAgentMessages(ctx); err != nil {
		t.Fatal(err)
	}
	if len(recovered.tasks) != 2 {
		t.Fatalf("recovered %d messages", len(recovered.tasks))
	}
	for _, msg := range recovered.tasks {
		if msg.ID == domainsubagent.AgentResultMessageID("task", "run-1") && !strings.Contains(msg.Content, "first result") {
			t.Fatalf("recovery replaced old result: %#v", msg)
		}
	}
}

func TestRecoverPendingParentMessagesSkipsChildMailboxEnvelopes(t *testing.T) {
	repository := memory.NewRepository()
	message := domainsubagent.AgentMessage{ID: "child-input", RecipientAgentID: "child-session",
		Kind: domainsubagent.AgentMessageKindInterAgent, Trigger: domainsubagent.AgentMessageTriggerQueue,
		Content: "new requirement", Status: domainsubagent.AgentMessagePending, CreatedAt: time.Now().UTC()}
	if err := repository.SaveAgentMessage(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	notifier := &recordingParentNotifier{}
	service := &Service{AgentMessages: repository, Tasks: repository, ParentNotifier: notifier}
	if err := service.RecoverPendingAgentMessages(context.Background()); err != nil {
		t.Fatal(err)
	}
	stored, err := repository.GetAgentMessage(context.Background(), message.ID)
	if err != nil || stored.Status != domainsubagent.AgentMessagePending || len(notifier.tasks) != 0 {
		t.Fatalf("child mailbox envelope was treated as a parent result: %#v, %#v, %v", stored, notifier.tasks, err)
	}
}

func (notifier *recordingParentNotifier) Notify(_ context.Context, task domainsubagent.AgentMessage) error {
	notifier.tasks = append(notifier.tasks, task)
	return nil
}

func TestParentCompletionNotificationIsIdempotentPerTask(t *testing.T) {
	notifier := &recordingParentNotifier{}
	service := &Service{ParentNotifier: notifier}
	task := domainsubagent.Task{ID: "task-1", ParentSessionID: "parent-1", Result: "done"}

	service.mu.Lock()
	service.notifyParentCompletionLocked(task)
	service.notifyParentCompletionLocked(task)
	service.mu.Unlock()

	if len(notifier.tasks) != 1 {
		t.Fatalf("notification count = %d, want 1", len(notifier.tasks))
	}
	if notifier.tasks[0].SourceTaskID != task.ID {
		t.Fatalf("notified task = %q, want %q", notifier.tasks[0].ID, task.ID)
	}
}

func TestParentCompletionClaimPreventsDuplicateRecovery(t *testing.T) {
	repository := memory.NewRepository()
	task := domainsubagent.Task{ID: "task-1", ParentSessionID: "parent-1", Status: domainsubagent.TaskStatusSucceeded, Result: "done"}
	if err := repository.SaveTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	message := domainsubagent.AgentMessage{ID: domainsubagent.AgentResultMessageID(task.ID, task.CurrentRunID), SourceTaskID: task.ID,
		RecipientAgentID: task.ParentSessionID, Kind: domainsubagent.AgentMessageKindTaskResult,
		Trigger: domainsubagent.AgentMessageTriggerQueue, Content: task.Result, Status: domainsubagent.AgentMessagePending, CreatedAt: time.Now().UTC()}
	if _, err := repository.EnqueueParentCompletion(context.Background(), task.ID, message); err != nil {
		t.Fatal(err)
	}
	first, err := repository.ClaimParentCompletion(context.Background(), message.ID, "owner-a", time.Now().UTC(), time.Minute)
	if err != nil || first.ClaimOwnerID != "owner-a" {
		t.Fatalf("first recovery claim failed: %#v, %v", first, err)
	}
	if _, err := repository.ClaimParentCompletion(context.Background(), message.ID, "owner-b", time.Now().UTC(), time.Minute); err == nil {
		t.Fatal("second recovery owner claimed an active parent completion")
	}
	if err := repository.CompleteParentCompletion(context.Background(), message.ID, "owner-a", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
}

func TestParentCompletionClaimExpiresAndCanBeTakenOver(t *testing.T) {
	repository := memory.NewRepository()
	task := domainsubagent.Task{ID: "task-expiring", ParentSessionID: "parent-1", Status: domainsubagent.TaskStatusSucceeded, Result: "done"}
	if err := repository.SaveTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	message := domainsubagent.AgentMessage{ID: domainsubagent.AgentResultMessageID(task.ID, task.CurrentRunID), SourceTaskID: task.ID,
		RecipientAgentID: task.ParentSessionID, Kind: domainsubagent.AgentMessageKindTaskResult,
		Trigger: domainsubagent.AgentMessageTriggerQueue, Content: task.Result, Status: domainsubagent.AgentMessagePending, CreatedAt: time.Now().UTC()}
	if _, err := repository.EnqueueParentCompletion(context.Background(), task.ID, message); err != nil {
		t.Fatal(err)
	}
	claimedAt := time.Now().UTC()
	if _, err := repository.ClaimParentCompletion(context.Background(), message.ID, "owner-a", claimedAt, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ClaimParentCompletion(context.Background(), message.ID, "owner-b", claimedAt.Add(2*time.Minute), time.Minute); err != nil {
		t.Fatalf("expired claim was not taken over: %v", err)
	}
	if err := repository.CompleteParentCompletion(context.Background(), message.ID, "owner-a", time.Now().UTC()); err == nil {
		t.Fatal("previous owner completed a claim after takeover")
	}
	if err := repository.ReleaseParentCompletion(context.Background(), message.ID, "owner-b", "retry"); err != nil {
		t.Fatalf("current owner could not release claim: %v", err)
	}
	if _, err := repository.ClaimParentCompletion(context.Background(), message.ID, "owner-c", time.Now().UTC(), time.Minute); err != nil {
		t.Fatalf("released message was not claimable: %v", err)
	}
}

func TestRecoverParentCompletionDoesNotAckBeforeParentConsumes(t *testing.T) {
	repository := memory.NewRepository()
	task := domainsubagent.Task{ID: "task-consume-later", ParentSessionID: "parent-1", Status: domainsubagent.TaskStatusSucceeded, Result: "done"}
	if err := repository.SaveTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	message := domainsubagent.AgentMessage{ID: domainsubagent.AgentResultMessageID(task.ID, task.CurrentRunID), SourceTaskID: task.ID,
		RecipientAgentID: task.ParentSessionID, Kind: domainsubagent.AgentMessageKindTaskResult,
		Trigger: domainsubagent.AgentMessageTriggerQueue, Content: task.Result, Status: domainsubagent.AgentMessagePending, CreatedAt: time.Now().UTC()}
	if _, err := repository.EnqueueParentCompletion(context.Background(), task.ID, message); err != nil {
		t.Fatal(err)
	}
	notifier := &recordingParentNotifier{}
	service := &Service{AgentMessages: repository, Tasks: repository, ParentNotifier: notifier, MessageOwnerID: "recovery-owner"}
	if err := service.RecoverPendingAgentMessages(context.Background()); err != nil {
		t.Fatal(err)
	}
	stored, err := repository.GetAgentMessage(context.Background(), message.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domainsubagent.AgentMessagePending {
		t.Fatalf("recovery acknowledged before parent consumption: %#v", stored)
	}
	if len(notifier.tasks) != 1 {
		t.Fatalf("parent notification count = %d, want 1", len(notifier.tasks))
	}
	if err := (AgentMessageAcknowledger{Repository: repository}).Acknowledge(message.ID); err != nil {
		t.Fatal(err)
	}
	stored, err = repository.GetAgentMessage(context.Background(), message.ID)
	if err != nil || stored.Status != domainsubagent.AgentMessageDelivered {
		t.Fatalf("parent acknowledgement did not complete message: %#v, %v", stored, err)
	}
}
