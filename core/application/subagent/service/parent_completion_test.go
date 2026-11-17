package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"myai/core/adapter/chat/pendinginput"
	"myai/core/adapter/subagent/memory"
	subagentcommand "myai/core/application/subagent/command"
	domainsubagent "myai/core/domain/subagent"
	domainworkspace "myai/core/domain/workspace"
	"myai/core/llm"
	subagentport "myai/core/port/subagent"
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

// Uses the production queue's complete-envelope validation and ACK path.
type queueCompletionParent struct {
	queue           *pendinginput.Queue
	notifyErr       error
	continueErr     error
	afterConsume    func()
	afterConsumeErr error
}

func (parent *queueCompletionParent) Notify(_ context.Context, message domainsubagent.AgentMessage) error {
	if parent.notifyErr != nil {
		return parent.notifyErr
	}
	return parent.queue.EnqueueAgentMessage(message.RecipientAgentID, message)
}
func (parent *queueCompletionParent) Continue(context.Context, subagentport.ParentContinuationRequest) (subagentport.ParentContinuationResult, error) {
	return subagentport.ParentContinuationResult{}, errors.New("expected pending continuation")
}
func (parent *queueCompletionParent) ContinuePending(_ context.Context, sessionID string, _ llm.ChatStreamHandler) (subagentport.ParentContinuationResult, error) {
	if parent.continueErr != nil {
		return subagentport.ParentContinuationResult{}, parent.continueErr
	}
	for _, item := range parent.queue.DrainIdentified(sessionID) {
		if err := parent.queue.Acknowledge(item.ID); err != nil {
			return subagentport.ParentContinuationResult{}, err
		}
	}
	if parent.afterConsume != nil {
		parent.afterConsume()
	}
	return subagentport.ParentContinuationResult{Content: "parent continued"}, parent.afterConsumeErr
}

func completionFixture(t *testing.T) (*Service, *memory.Repository, *queueCompletionParent, domainsubagent.Task) {
	t.Helper()
	repository := memory.NewRepository()
	queue := pendinginput.NewQueue()
	queue.SetAcknowledger(AgentMessageAcknowledger{Repository: repository, OwnerID: "parent-test"})
	parent := &queueCompletionParent{queue: queue}
	now := time.Now().UTC()
	task := domainsubagent.Task{ID: "task-budget", CurrentRunID: "run-1", ParentSessionID: "parent-budget", ChildSessionID: "child-budget",
		Status: domainsubagent.TaskStatusSucceeded, Result: "full result", Unread: true, UpdatedAt: now, CompletedAt: &now}
	if err := repository.SaveTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	return &Service{Tasks: repository, Runs: repository, AgentMessages: repository, ParentNotifier: parent, ParentContinuation: parent}, repository, parent, task
}

func decodeCompletionReport(t *testing.T, content string) map[string]any {
	t.Helper()
	start := strings.Index(content, "<subagent_report>\n") + len("<subagent_report>\n")
	end := strings.LastIndex(content, "\n</subagent_report>")
	if start < len("<subagent_report>\n") || end <= start {
		t.Fatal("completion report framing is missing")
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(content[start:end]), &report); err != nil {
		t.Fatal(err)
	}
	return report
}

func TestParentWaitDeliversBoundedFullEnvelopeAndPreservesFullResult(t *testing.T) {
	for _, test := range []struct {
		name, result string
		metadata     bool
	}{
		{"english", strings.Repeat("long result ", 2000), false},
		{"chinese", strings.Repeat("完整任务结果", 3000), false},
		{"unicode", strings.Repeat("é界𐐷é", 3000), false},
		{"invalid utf8", strings.Repeat("valid\xff", 3000), false},
		{"json escaping", strings.Repeat("\x01\n\t\\\"", 3000), false},
		{"large metadata", strings.Repeat("result", 3000), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, repository, parent, task := completionFixture(t)
			task.Result, task.ErrorMessage = test.result, strings.Repeat("error details ", 1500)
			if test.metadata {
				task.ID, task.CurrentRunID = strings.Repeat("task-", 2000), strings.Repeat("run-", 2000)
				task.Title = strings.Repeat("长标题", 10000)
				task.ChangeSet.Status = domainworkspace.ChangeSetStatusPending
				for range 203 {
					task.ChangeSet.Files = append(task.ChangeSet.Files, domainworkspace.FileChange{Path: strings.Repeat("文件路径", 1000)})
				}
			}
			if err := repository.SaveTaskAndRun(context.Background(), task, domainsubagent.Run{ID: task.CurrentRunID, TaskID: task.ID, Result: task.Result}); err != nil {
				t.Fatal(err)
			}
			wait, err := svc.Wait(context.Background(), subagentcommand.WaitTask{TaskID: task.ID, ParentSessionID: task.ParentSessionID})
			if err != nil || !wait.ResultsInMailbox {
				t.Fatalf("parent wait did not complete with a queued result: %v", err)
			}
			items := parent.queue.DrainIdentified(task.ParentSessionID)
			if len(items) != 1 {
				t.Fatalf("queued results = %d, want 1", len(items))
			}
			item := items[0]
			if item.SourceTaskID != task.ID || item.ID != domainsubagent.AgentResultMessageID(task.ID, task.CurrentRunID) {
				t.Fatal("delivery lost its stable source identity")
			}
			if utf8.RuneCountInString(item.Content) > pendinginput.MaxPendingRunes || !utf8.ValidString(item.Content) {
				t.Fatal("complete serialized delivery exceeds the real queue budget or contains invalid UTF-8")
			}
			report := decodeCompletionReport(t, item.Content)
			if report["result_truncated"] != true || !strings.Contains(report["full_result"].(string), "check_async_task") {
				t.Fatal("summary lacks omission metadata or a full result entry point")
			}
			persisted, _ := repository.GetTask(context.Background(), task.ID)
			if !persisted.Unread || persisted.Result != test.result {
				t.Fatal("queueing consumed or shortened the full task result")
			}
			if err := parent.queue.Acknowledge(item.ID); err != nil {
				t.Fatal(err)
			}
			persisted, _ = repository.GetTask(context.Background(), task.ID)
			if persisted.Unread || persisted.Result != test.result {
				t.Fatal("successful automatic ACK did not consume exactly the saved result")
			}
			runs, _ := repository.ListRuns(context.Background(), task.ID)
			if len(runs) != 1 || runs[0].Result != test.result {
				t.Fatal("delivery shortened the durable run result")
			}
		})
	}
}

func TestParentCompletionACKIsRevisionScopedAndIdempotent(t *testing.T) {
	for _, kind := range []string{"review only", "new run", "new result", "new completion", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			svc, repository, parent, task := completionFixture(t)
			if err := svc.ensureParentCompletionQueued(task); err != nil {
				t.Fatal(err)
			}
			item := parent.queue.DrainIdentified(task.ParentSessionID)[0]
			current := domainsubagent.CloneTask(task)
			current.UpdatedAt = current.UpdatedAt.Add(time.Second)
			switch kind {
			case "new run":
				current.CurrentRunID = "run-2"
			case "new result":
				current.Result = "a newer full result"
			case "new completion":
				completed := task.CompletedAt.Add(time.Second)
				current.CompletedAt = &completed
			case "canceled":
				current.Status, current.Unread = domainsubagent.TaskStatusCanceled, false
			}
			if err := repository.SaveTask(context.Background(), current); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err := parent.queue.Acknowledge(item.ID); err != nil {
					t.Fatal(err)
				}
			}
			persisted, _ := repository.GetTask(context.Background(), task.ID)
			wantUnread := kind != "review only" && kind != "canceled"
			if persisted.Unread != wantUnread {
				t.Fatal("ACK changed the wrong completion revision's unread projection")
			}
			// A later duplicate of a successful ACK must not clear any new result.
			persisted.CurrentRunID, persisted.Result, persisted.Unread = "latest-run", "latest result", true
			if err := repository.SaveTask(context.Background(), persisted); err != nil {
				t.Fatal(err)
			}
			if err := parent.queue.Acknowledge(item.ID); err != nil {
				t.Fatal(err)
			}
			persisted, _ = repository.GetTask(context.Background(), task.ID)
			if !persisted.Unread {
				t.Fatal("duplicate old ACK consumed a newer result")
			}
		})
	}
}

func TestParentCompletionConcurrentACKsCanRetryAcrossOwners(t *testing.T) {
	svc, repository, _, task := completionFixture(t)
	if err := svc.ensureParentCompletionQueued(task); err != nil {
		t.Fatal(err)
	}
	messageID := domainsubagent.AgentResultMessageID(task.ID, task.CurrentRunID)
	var wg sync.WaitGroup
	for index := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = (AgentMessageAcknowledger{Repository: repository, OwnerID: fmt.Sprintf("instance-%d", index)}).Acknowledge(messageID)
		}()
	}
	wg.Wait()
	if err := (AgentMessageAcknowledger{Repository: repository, OwnerID: "final-retry"}).Acknowledge(messageID); err != nil {
		t.Fatal(err)
	}
	stored, _ := repository.GetAgentMessage(context.Background(), messageID)
	persisted, _ := repository.GetTask(context.Background(), task.ID)
	if stored.Status != domainsubagent.AgentMessageDelivered || persisted.Unread {
		t.Fatal("concurrent ACKs did not converge")
	}
}

type failingCompletionRepository struct {
	*memory.Repository
	fail bool
}

func (repository *failingCompletionRepository) CompleteParentCompletion(ctx context.Context, messageID, ownerID string, at time.Time) error {
	if repository.fail {
		return errors.New("temporary commit failure")
	}
	return repository.Repository.CompleteParentCompletion(ctx, messageID, ownerID, at)
}

func TestParentCompletionACKFailureReleasesClaimForRetry(t *testing.T) {
	svc, repository, _, task := completionFixture(t)
	if err := svc.ensureParentCompletionQueued(task); err != nil {
		t.Fatal(err)
	}
	fault := &failingCompletionRepository{Repository: repository, fail: true}
	ack := AgentMessageAcknowledger{Repository: fault, OwnerID: "instance-1"}
	messageID := domainsubagent.AgentResultMessageID(task.ID, task.CurrentRunID)
	if err := ack.Acknowledge(messageID); err == nil {
		t.Fatal("expected failed ACK")
	}
	stored, _ := repository.GetAgentMessage(context.Background(), messageID)
	persisted, _ := repository.GetTask(context.Background(), task.ID)
	if stored.Status != domainsubagent.AgentMessagePending || stored.ClaimOwnerID != "" || !persisted.Unread {
		t.Fatal("failed ACK lost retryable delivery or unread state")
	}
	fault.fail = false
	if err := ack.Acknowledge(messageID); err != nil {
		t.Fatal(err)
	}
	persisted, _ = repository.GetTask(context.Background(), task.ID)
	if persisted.Unread {
		t.Fatal("retried ACK did not clear unread")
	}
}

func TestParentCompletionQueueFailureCanRetryWithoutLosingUnread(t *testing.T) {
	svc, repository, parent, task := completionFixture(t)
	for range pendinginput.MaxPendingMessages {
		if err := parent.queue.Enqueue(task.ParentSessionID, "queued user input"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Wait(context.Background(), subagentcommand.WaitTask{TaskID: task.ID}); err == nil {
		t.Fatal("expected full queue error")
	}
	messageID := domainsubagent.AgentResultMessageID(task.ID, task.CurrentRunID)
	stored, _ := repository.GetAgentMessage(context.Background(), messageID)
	persisted, _ := repository.GetTask(context.Background(), task.ID)
	if stored.Status != domainsubagent.AgentMessagePending || !persisted.Unread {
		t.Fatal("notification failure consumed a result")
	}
	parent.queue.Drain(task.ParentSessionID)
	if _, err := svc.Wait(context.Background(), subagentcommand.WaitTask{TaskID: task.ID}); err != nil {
		t.Fatal(err)
	}
	if len(parent.queue.DrainIdentified(task.ParentSessionID)) != 1 {
		t.Fatal("retry did not enqueue exactly one result")
	}
}

func TestExplicitResumeUsesSameACKAndPreservesNewerResults(t *testing.T) {
	for _, kind := range []string{"success", "notification failure", "continuation failure", "failure after ACK", "new revision after ACK"} {
		t.Run(kind, func(t *testing.T) {
			svc, repository, parent, task := completionFixture(t)
			switch kind {
			case "notification failure":
				parent.notifyErr = errors.New("temporary enqueue failure")
			case "continuation failure":
				parent.continueErr = errors.New("temporary model failure")
			case "failure after ACK":
				parent.afterConsumeErr = errors.New("later parent operation failed")
			case "new revision after ACK":
				parent.afterConsume = func() {
					current, _ := repository.GetTask(context.Background(), task.ID)
					current.CurrentRunID, current.Result, current.Unread = "run-2", "new completion", true
					if err := repository.SaveTask(context.Background(), current); err != nil {
						t.Fatal(err)
					}
				}
			}
			_, err := svc.Resume(context.Background(), subagentcommand.ResumeTask{TaskID: task.ID, ParentSessionID: task.ParentSessionID})
			wantErr := strings.Contains(kind, "failure")
			if (err != nil) != wantErr {
				t.Fatalf("resume error = %v", err)
			}
			current, _ := repository.GetTask(context.Background(), task.ID)
			wantUnread := kind == "notification failure" || kind == "continuation failure" || kind == "new revision after ACK"
			if current.Unread != wantUnread {
				t.Fatal("explicit resume changed the wrong unread result")
			}
			if kind == "notification failure" || kind == "continuation failure" {
				parent.notifyErr, parent.continueErr = nil, nil
				if _, err := svc.Resume(context.Background(), subagentcommand.ResumeTask{TaskID: task.ID}); err != nil {
					t.Fatal(err)
				}
				current, _ = repository.GetTask(context.Background(), task.ID)
				if current.Unread {
					t.Fatal("retried explicit resume did not clear unread")
				}
			}
		})
	}
}

func TestRecoverOversizedLegacyCompletionUsesSavedRunAndPersistsSummary(t *testing.T) {
	svc, repository, parent, task := completionFixture(t)
	data, _ := json.Marshal(map[string]any{"task_id": task.ID, "result": strings.Repeat("old saved result ", 2000), "status": "succeeded"})
	message := domainsubagent.AgentMessage{ID: domainsubagent.AgentResultMessageID(task.ID, task.CurrentRunID), SourceTaskID: task.ID,
		RecipientAgentID: task.ParentSessionID, Kind: domainsubagent.AgentMessageKindTaskResult, Trigger: domainsubagent.AgentMessageTriggerTurn,
		Status: domainsubagent.AgentMessagePending, CreatedAt: time.Now().UTC(), Content: "legacy instructions\n<subagent_report>\n" + string(data) + "\n</subagent_report>"}
	if err := repository.SaveAgentMessage(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	task.CurrentRunID, task.Result = "run-2", "newer task result must not replace old content"
	if err := repository.SaveTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := svc.RecoverPendingAgentMessages(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	items := parent.queue.DrainIdentified(task.ParentSessionID)
	if len(items) != 1 || !strings.Contains(items[0].Content, "old saved result") || strings.Contains(items[0].Content, "newer task result") {
		t.Fatal("legacy recovery changed message identity or payload across task runs")
	}
	stored, _ := repository.GetAgentMessage(context.Background(), message.ID)
	if stored.Content != items[0].Content || utf8.RuneCountInString(stored.Content) > pendinginput.MaxPendingRunes {
		t.Fatal("legacy recovery did not persist the exact bounded delivery")
	}
	if err := parent.queue.Acknowledge(message.ID); err != nil {
		t.Fatal(err)
	}
	current, _ := repository.GetTask(context.Background(), task.ID)
	if !current.Unread {
		t.Fatal("legacy ACK consumed a newer run")
	}
}
