package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	historyrepository "myai/core/adapter/persistence/sqlite/history/repository"
	subagentevents "myai/core/adapter/subagent/events"
	"myai/core/adapter/subagent/memory"
	"myai/core/adapter/workspace/snapshot"
	subagentcommand "myai/core/application/subagent/command"
	domainsubagent "myai/core/domain/subagent"
	domainworkspace "myai/core/domain/workspace"
	subagentport "myai/core/port/subagent"
	workspaceport "myai/core/port/workspace"
)

func TestWaitRegistrationRetainsWakeAndOtherWaiters(t *testing.T) {
	repository := memory.NewRepository()
	parent := domainsubagent.Task{ID: "parent", ChildSessionID: "child-session", CurrentRunID: "run-1", Status: domainsubagent.TaskStatusRunning}
	if err := repository.SaveTask(context.Background(), parent); err != nil {
		t.Fatal(err)
	}
	service := &Service{Tasks: repository, IDs: &sequenceIDs{}}
	command := subagentcommand.WaitTask{ParentTaskID: parent.ID, ParentSessionID: parent.ChildSessionID}
	first, err := service.markWaitingForChildren(command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.markWaitingForChildren(command)
	if err != nil {
		t.Fatal(err)
	}
	service.restoreRunningAfterChildren(parent.ID, first)
	current, _ := repository.GetTask(context.Background(), parent.ID)
	if current.Status != domainsubagent.TaskStatusWaitingSubagents {
		t.Fatal("one departing waiter restored running while another was waiting")
	}
	// Send before reading the registered channel, including a second send.
	for i := 0; i < 2; i++ {
		if _, err := service.SendMessage(context.Background(), subagentcommand.SendMessage{TaskID: parent.ID, Content: "new input"}); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-second.wakeup:
	default:
		t.Fatal("mailbox wake was lost between registration and channel use")
	}
	service.restoreRunningAfterChildren(parent.ID, second)
	current, _ = repository.GetTask(context.Background(), parent.ID)
	if current.Status != domainsubagent.TaskStatusRunning || len(service.waitingWakeups) != 0 {
		t.Fatalf("last waiter did not restore/clean up: %#v", current)
	}
}

func TestWaitWakesForPreviouslyQueuedMailbox(t *testing.T) {
	repository := memory.NewRepository()
	parent := domainsubagent.Task{ID: "parent", ChildSessionID: "session", Status: domainsubagent.TaskStatusRunning,
		Mailbox: []domainsubagent.Message{{ID: "message", Content: "new requirement", Status: domainsubagent.MessageStatusPending}}}
	child := domainsubagent.Task{ID: "child", ParentSessionID: "session", Status: domainsubagent.TaskStatusRunning}
	for _, task := range []domainsubagent.Task{parent, child} {
		if err := repository.SaveTask(context.Background(), task); err != nil {
			t.Fatal(err)
		}
	}
	service := &Service{Tasks: repository, Events: subagentevents.NewBus()}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := service.Wait(ctx, subagentcommand.WaitTask{TaskID: child.ID, ParentTaskID: parent.ID, ParentSessionID: "session", Timeout: time.Second})
	if err != nil || !result.WokenByMailbox || result.TimedOut {
		t.Fatalf("pre-existing input did not wake wait: %#v, %v", result, err)
	}
}

func TestWaitDoesNotWakeForDeliveringMessageOrRestoreAnotherRun(t *testing.T) {
	repository := memory.NewRepository()
	parent := domainsubagent.Task{ID: "parent", ChildSessionID: "session", CurrentRunID: "old", Status: domainsubagent.TaskStatusRunning,
		Mailbox: []domainsubagent.Message{{ID: "message", Status: domainsubagent.MessageStatusDelivering}}}
	if err := repository.SaveTask(context.Background(), parent); err != nil {
		t.Fatal(err)
	}
	service := &Service{Tasks: repository}
	waiter, err := service.markWaitingForChildren(subagentcommand.WaitTask{ParentTaskID: parent.ID, ParentSessionID: "session"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-waiter.wakeup:
		t.Fatal("current mailbox turn woke itself")
	default:
	}
	parent.CurrentRunID = "new"
	parent.Status = domainsubagent.TaskStatusWaitingSubagents
	if err := repository.SaveTask(context.Background(), parent); err != nil {
		t.Fatal(err)
	}
	service.restoreRunningAfterChildren(parent.ID, waiter)
	current, _ := repository.GetTask(context.Background(), parent.ID)
	if current.Status != domainsubagent.TaskStatusWaitingSubagents {
		t.Fatal("old waiter changed the new run")
	}
}

func completedFollowupService(t *testing.T) (*Service, *memory.Repository, domainsubagent.Task) {
	t.Helper()
	repository := memory.NewRepository()
	task := domainsubagent.Task{ID: "child-task", ParentSessionID: "parent", ChildSessionID: "session", CurrentRunID: "first-run",
		Status: domainsubagent.TaskStatusSucceeded, Unread: true, Result: "initial result",
		Workspace: domainworkspace.Reference{Mode: domainworkspace.IsolationModeDirect}}
	run := domainsubagent.Run{ID: task.CurrentRunID, TaskID: task.ID, Sequence: 1, Status: domainsubagent.RunStatusSucceeded}
	if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
		t.Fatal(err)
	}
	return &Service{Tasks: repository, Runs: repository, IDs: &sequenceIDs{}, Scheduler: &fakeScheduler{}}, repository, task
}

func TestFollowupWaitsForPreviousExecutionCleanup(t *testing.T) {
	service, repository, task := completedFollowupService(t)
	service.registerActiveRun(task.ID, task.CurrentRunID)
	base, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ctx := &observedDoneContext{Context: base, observed: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		_, err := service.Followup(ctx, subagentcommand.FollowupTask{TaskID: task.ID, Content: "continue"})
		result <- err
	}()
	select {
	case <-ctx.observed:
	case <-base.Done():
		t.Fatal("follow-up did not wait for cleanup")
	}
	current, _ := repository.GetTask(context.Background(), task.ID)
	if current.CurrentRunID != task.CurrentRunID {
		t.Fatal("follow-up mutated the task before cleanup completed")
	}
	service.completeActiveRun(task.ID, task.CurrentRunID)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	current, _ = repository.GetTask(context.Background(), task.ID)
	service.completeActiveRun(task.ID, task.CurrentRunID)
	if !service.taskIsActive(task.ID) || current.CurrentRunID == task.CurrentRunID {
		t.Fatal("old cleanup removed the new execution")
	}
}

type observedDoneContext struct {
	context.Context
	once     sync.Once
	observed chan struct{}
}

func (ctx *observedDoneContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.observed) })
	return ctx.Context.Done()
}

func TestFollowupRequestIDSurvivesServiceRestart(t *testing.T) {
	service, repository, task := completedFollowupService(t)
	command := subagentcommand.FollowupTask{TaskID: task.ID, ParentSessionID: task.ParentSessionID, RequestID: "followup-1", Content: "continue"}
	first, err := service.Followup(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := repository.ListRuns(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Restart recovery substitutes a continuation instruction. Request
	// identity must still refer to the original input rather than this text.
	runs[1].Instruction = interruptedTaskContinuation
	if err := repository.SaveRun(context.Background(), runs[1]); err != nil {
		t.Fatal(err)
	}
	scheduler := &fakeScheduler{}
	restarted := &Service{Tasks: repository, Runs: repository, IDs: &sequenceIDs{}, Scheduler: scheduler}
	retry, err := restarted.Followup(context.Background(), command)
	if err != nil || retry.Value.CurrentRunID != first.Value.CurrentRunID || scheduler.submissions != 0 {
		t.Fatalf("retry admitted duplicate work: %#v, %v", retry, err)
	}
	command.Content = "different input"
	if _, err := restarted.Followup(context.Background(), command); err == nil {
		t.Fatal("request ID with conflicting content was accepted")
	}
	runs, _ = repository.ListRuns(context.Background(), task.ID)
	if len(runs) != 2 {
		t.Fatalf("expected two runs, got %d", len(runs))
	}
}

func TestFollowupScheduleFailureReturnsPersistedFailure(t *testing.T) {
	service, repository, task := completedFollowupService(t)
	service.Scheduler = failingScheduler{}
	result, err := service.Followup(context.Background(), subagentcommand.FollowupTask{TaskID: task.ID, Content: "continue"})
	current, _ := repository.GetTask(context.Background(), task.ID)
	if err == nil || result.Value.Status != domainsubagent.TaskStatusFailed || result.Value.CurrentRunID != current.CurrentRunID || service.taskIsActive(task.ID) {
		t.Fatalf("schedule failure returned stale state or leaked ownership: %#v, %v", result, err)
	}
}

func TestFollowupRejectsOversizedInputWithoutMutation(t *testing.T) {
	service, repository, task := completedFollowupService(t)
	_, err := service.Followup(context.Background(), subagentcommand.FollowupTask{TaskID: task.ID, Content: strings.Repeat("a", domainsubagent.MaxInstructionRunes+1)})
	current, _ := repository.GetTask(context.Background(), task.ID)
	if err == nil || current.CurrentRunID != task.CurrentRunID || !current.Unread {
		t.Fatal("oversized follow-up changed the completed task")
	}
}

func TestStaleRunCannotStartOrFinishCurrentTask(t *testing.T) {
	service, repository, task := completedFollowupService(t)
	oldRun := domainsubagent.Run{ID: task.CurrentRunID, TaskID: task.ID}
	_, err := service.Followup(context.Background(), subagentcommand.FollowupTask{TaskID: task.ID, Content: "continue"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := service.beginExecution(context.Background(), task.ID, oldRun.ID); err != nil || ok {
		t.Fatalf("stale run started: %v", err)
	}
	if err := service.finishSuccess(task, oldRun, subagentport.AgentRunResult{Content: "stale"}, domainworkspace.ChangeSet{}); err != nil {
		t.Fatal(err)
	}
	if err := service.finishError(task, oldRun, context.Background(), errors.New("stale error")); err != nil {
		t.Fatal(err)
	}
	current, _ := repository.GetTask(context.Background(), task.ID)
	if current.Status != domainsubagent.TaskStatusQueued || current.Result != "" || current.ErrorMessage != "" {
		t.Fatalf("stale completion corrupted current run: %#v", current)
	}
}

func TestFollowupUsesDistinctSchedulerIdentity(t *testing.T) {
	service, _, task := completedFollowupService(t)
	scheduler := &retainedScheduler{jobs: map[string]subagentport.ScheduledTask{task.CurrentRunID: nil, task.ID: nil}}
	service.Scheduler = scheduler
	result, err := service.Followup(context.Background(), subagentcommand.FollowupTask{TaskID: task.ID, Content: "continue"})
	if err != nil || scheduler.jobs[result.Value.CurrentRunID] == nil {
		t.Fatalf("new run collided with draining scheduler entry: %v", err)
	}
	service.completeActiveRun(task.ID, result.Value.CurrentRunID)
	if _, err := service.Cancel(context.Background(), subagentcommand.CancelTask{TaskID: task.ID}); err != nil {
		t.Fatal(err)
	}
	if scheduler.canceled != result.Value.CurrentRunID {
		t.Fatalf("cancel targeted %q instead of current run", scheduler.canceled)
	}
}

type retainedScheduler struct {
	jobs     map[string]subagentport.ScheduledTask
	canceled string
}

func (scheduler *retainedScheduler) Submit(id string, run subagentport.ScheduledTask) error {
	if _, exists := scheduler.jobs[id]; exists {
		return errors.New("already scheduled")
	}
	scheduler.jobs[id] = run
	return nil
}
func (scheduler *retainedScheduler) Cancel(id string) bool {
	scheduler.canceled = id
	return true
}

func TestWorkspacePreparationPreservesConcurrentMailboxAndCancellation(t *testing.T) {
	for _, cancelTask := range []bool{false, true} {
		t.Run(map[bool]string{false: "mailbox", true: "cancel"}[cancelTask], func(t *testing.T) {
			service, repository, task := completedFollowupService(t)
			task.Status = domainsubagent.TaskStatusRunning
			task.Workspace = domainworkspace.Reference{ID: "workspace", Mode: domainworkspace.IsolationModeSnapshot, Root: "source"}
			if err := repository.SaveTask(context.Background(), task); err != nil {
				t.Fatal(err)
			}
			service.Workspaces = &preparationHook{hook: func() {
				if _, err := service.SendMessage(context.Background(), subagentcommand.SendMessage{TaskID: task.ID, Content: "new input"}); err != nil {
					t.Fatal(err)
				}
				if cancelTask {
					if _, err := service.Cancel(context.Background(), subagentcommand.CancelTask{TaskID: task.ID}); err != nil {
						t.Fatal(err)
					}
				}
			}}
			_, err := service.prepareWorkspace(context.Background(), task)
			if cancelTask != errors.Is(err, context.Canceled) {
				t.Fatalf("unexpected preparation error: %v", err)
			}
			current, _ := repository.GetTask(context.Background(), task.ID)
			if len(current.Mailbox) != 1 || (cancelTask && current.Status != domainsubagent.TaskStatusCanceled) {
				t.Fatalf("workspace preparation overwrote concurrent update: %#v", current)
			}
		})
	}
}

type preparationHook struct {
	fakeWorkspaceManager
	hook func()
}

func (manager *preparationHook) Prepare(ctx context.Context, request workspaceport.PrepareRequest) (workspaceport.PreparedWorkspace, error) {
	manager.hook()
	return manager.fakeWorkspaceManager.Prepare(ctx, request)
}

func TestPreparedOpenSandboxIsReused(t *testing.T) {
	service := &Service{Workspaces: &preparationHook{hook: func() { t.Fatal("existing sandbox was prepared again") }}}
	task := domainsubagent.Task{Workspace: domainworkspace.Reference{ID: "workspace", Mode: domainworkspace.IsolationModeOpenSandbox, Root: "mirror", SandboxID: "existing"}}
	prepared, err := service.prepareWorkspace(context.Background(), task)
	if err != nil || prepared.Workspace != task.Workspace {
		t.Fatalf("sandbox identity changed: %#v, %v", prepared.Workspace, err)
	}
}

func TestFollowupChecksRealSnapshotLifecycle(t *testing.T) {
	for _, lifecycle := range []string{"pending", "applied", "discarded", "failed"} {
		t.Run(lifecycle, func(t *testing.T) {
			service, repository, task := completedFollowupService(t)
			manager, err := snapshot.New(t.TempDir(), historyrepository.Factory{})
			if err != nil {
				t.Fatal(err)
			}
			source := t.TempDir()
			prepared, err := manager.Prepare(context.Background(), workspaceport.PrepareRequest{WorkspaceID: "snapshot", Mode: domainworkspace.IsolationModeSnapshot, SourceRoot: source})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(prepared.Reference.Root, "result.txt"), []byte("first turn"), 0o600); err != nil {
				t.Fatal(err)
			}
			task.Workspace = prepared.Reference
			task.ChangeSet = domainworkspace.ChangeSet{WorkspaceID: "snapshot", Status: domainworkspace.ChangeSetStatusPending}
			switch lifecycle {
			case "applied":
				_, err = manager.Apply(context.Background(), workspaceport.ApplyRequest{Reference: prepared.Reference})
			case "discarded":
				_, err = manager.Discard(context.Background(), workspaceport.DiscardRequest{Reference: prepared.Reference})
			case "failed":
				task.Status = domainsubagent.TaskStatusFailed
			}
			if err != nil {
				t.Fatal(err)
			}
			// Keep a stale pending task record even when the manager has closed it.
			if err := repository.SaveTask(context.Background(), task); err != nil {
				t.Fatal(err)
			}
			service.Workspaces = manager
			service.Sessions = &fakeChildSessions{}
			service.Runner = fakeRunner{}
			task.Definition.TimeoutSeconds = 10
			if err := repository.SaveTask(context.Background(), task); err != nil {
				t.Fatal(err)
			}
			_, err = service.Followup(context.Background(), subagentcommand.FollowupTask{TaskID: task.ID, Content: "continue"})
			if lifecycle != "pending" {
				current, _ := repository.GetTask(context.Background(), task.ID)
				if err == nil || current.CurrentRunID != task.CurrentRunID || !current.Unread {
					t.Fatalf("closed workspace admitted follow-up: %#v, %v", current, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			service.Scheduler.(*fakeScheduler).task(context.Background())
			current, _ := repository.GetTask(context.Background(), task.ID)
			if current.Status != domainsubagent.TaskStatusSucceeded || current.Workspace != prepared.Reference || len(current.ChangeSet.Files) != 1 {
				t.Fatalf("pending snapshot was not preserved: %#v", current)
			}
		})
	}
}
