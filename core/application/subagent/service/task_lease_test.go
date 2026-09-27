package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"myai/core/adapter/subagent/memory"
	subagentcommand "myai/core/application/subagent/command"
	domainsubagent "myai/core/domain/subagent"
	domainworkspace "myai/core/domain/workspace"
	modelport "myai/core/port/model"
	subagentport "myai/core/port/subagent"
)

func TestFollowupRequiresCrossProcessExecutionLease(t *testing.T) {
	service, repository, task := completedFollowupService(t)
	service.ExecutionLease = repository
	service.ExecutionOwnerID = "instance-b"
	if err := repository.Acquire(context.Background(), task.ID, task.CurrentRunID, "instance-a", time.Minute); err != nil {
		t.Fatal(err)
	}
	_, err := service.Followup(context.Background(), subagentcommand.FollowupTask{TaskID: task.ID, Content: "continue"})
	if !errors.Is(err, subagentport.ErrExecutionLeaseNotAcquired) {
		t.Fatalf("follow-up did not reject another owner's lease: %v", err)
	}
	current, err := repository.GetTask(context.Background(), task.ID)
	if err != nil || current.CurrentRunID != task.CurrentRunID || current.Status != domainsubagent.TaskStatusSucceeded {
		t.Fatalf("lease conflict changed persisted task: %#v, %v", current, err)
	}
	if err := repository.Release(context.Background(), task.ID, task.CurrentRunID, "instance-a"); err != nil {
		t.Fatal(err)
	}
	admitted, err := service.Followup(context.Background(), subagentcommand.FollowupTask{TaskID: task.ID, Content: "continue"})
	if err != nil || admitted.Value.CurrentRunID == task.CurrentRunID {
		t.Fatalf("follow-up did not claim released task: %#v, %v", admitted, err)
	}
	other := &Service{Tasks: repository, Runs: repository, IDs: &sequenceIDs{}, Scheduler: &fakeScheduler{},
		ExecutionLease: repository, ExecutionOwnerID: "instance-c"}
	_, err = other.Followup(context.Background(), subagentcommand.FollowupTask{TaskID: task.ID, Content: "another"})
	if err == nil {
		t.Fatal("second instance admitted an active turn")
	}
}

func TestLeaseGuardedCommitRejectsStaleOwner(t *testing.T) {
	repository := memory.NewRepository()
	task := domainsubagent.Task{ID: "task", CurrentRunID: "run", Status: domainsubagent.TaskStatusQueued}
	run := domainsubagent.Run{ID: "run", TaskID: task.ID, Status: domainsubagent.RunStatusQueued}
	if err := repository.Acquire(context.Background(), task.ID, run.ID, "first", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveOwnedTaskAndRun(context.Background(), task, run, "first", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := repository.Release(context.Background(), task.ID, run.ID, "first"); err != nil {
		t.Fatal(err)
	}
	if err := repository.Acquire(context.Background(), task.ID, run.ID, "second", time.Minute); err != nil {
		t.Fatal(err)
	}
	task.Status = domainsubagent.TaskStatusFailed
	run.Status = domainsubagent.RunStatusFailed
	if err := repository.SaveOwnedTaskAndRun(context.Background(), task, run, "first", time.Minute); !errors.Is(err, subagentport.ErrExecutionLeaseNotAcquired) {
		t.Fatalf("stale owner committed after lease transfer: %v", err)
	}
	current, err := repository.GetTask(context.Background(), task.ID)
	if err != nil || current.Status != domainsubagent.TaskStatusQueued {
		t.Fatalf("stale write changed task: %#v, %v", current, err)
	}
}

func TestLeaseGuardedSuccessRetainsConcurrentChildInput(t *testing.T) {
	repository := memory.NewRepository()
	task := domainsubagent.Task{ID: "task", ParentSessionID: "parent", ChildSessionID: "child", CurrentRunID: "run", Status: domainsubagent.TaskStatusRunning}
	run := domainsubagent.Run{ID: "run", TaskID: task.ID, Status: domainsubagent.RunStatusRunning}
	if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
		t.Fatal(err)
	}
	if err := repository.Acquire(context.Background(), task.ID, run.ID, "owner", time.Minute); err != nil {
		t.Fatal(err)
	}
	message := domainsubagent.AgentMessage{ID: "message", RecipientAgentID: task.ChildSessionID,
		Kind: domainsubagent.AgentMessageKindInterAgent, Trigger: domainsubagent.AgentMessageTriggerQueue,
		Content: "new input", Status: domainsubagent.AgentMessagePending, CreatedAt: time.Now().UTC()}
	if _, err := repository.EnqueueChildAgentMessage(context.Background(), task.ID, message); err != nil {
		t.Fatal(err)
	}
	task.Status = domainsubagent.TaskStatusSucceeded
	run.Status = domainsubagent.RunStatusSucceeded
	if err := repository.SaveOwnedTaskAndRun(context.Background(), task, run, "owner", time.Minute); !errors.Is(err, subagentport.ErrPendingMailbox) {
		t.Fatalf("success commit overwrote concurrent mailbox input: %v", err)
	}
	current, err := repository.GetTask(context.Background(), task.ID)
	if err != nil || current.Status != domainsubagent.TaskStatusRunning || len(current.Mailbox) != 1 {
		t.Fatalf("mailbox was lost in terminal transition: %#v, %v", current, err)
	}
}

func TestRecoverySkipsLiveLeaseAndRetriesAfterRelease(t *testing.T) {
	repository := memory.NewRepository()
	task := domainsubagent.Task{ID: "task", CurrentRunID: "run", Status: domainsubagent.TaskStatusRunning}
	run := domainsubagent.Run{ID: "run", TaskID: task.ID, Status: domainsubagent.RunStatusRunning}
	if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
		t.Fatal(err)
	}
	if err := repository.Acquire(context.Background(), task.ID, run.ID, "live-instance", time.Minute); err != nil {
		t.Fatal(err)
	}
	scheduler := &fakeScheduler{}
	service := &Service{Tasks: repository, Runs: repository, TaskRuns: repository, Scheduler: scheduler,
		ExecutionLease: repository, ExecutionOwnerID: "recovery-instance"}
	if err := service.RecoverInterruptedTasks(context.Background()); err != nil {
		t.Fatal(err)
	}
	if scheduler.submissions != 0 {
		t.Fatal("recovery scheduled a task owned by a live instance")
	}
	if err := repository.Release(context.Background(), task.ID, run.ID, "live-instance"); err != nil {
		t.Fatal(err)
	}
	if err := service.RecoverInterruptedTasks(context.Background()); err != nil {
		t.Fatal(err)
	}
	if scheduler.submissions != 1 {
		t.Fatalf("recovery did not schedule released task: %d", scheduler.submissions)
	}
}

func TestSuccessfulTurnPersistsChildUsageSeparately(t *testing.T) {
	repository := memory.NewRepository()
	task := domainsubagent.Task{ID: "task", CurrentRunID: "run", Status: domainsubagent.TaskStatusRunning,
		Usage: modelport.TokenUsage{TotalTokens: 10, Available: true}}
	run := domainsubagent.Run{ID: "run", TaskID: task.ID, Status: domainsubagent.RunStatusRunning}
	if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
		t.Fatal(err)
	}
	service := &Service{Tasks: repository, Runs: repository}
	usage := modelport.TokenUsage{PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10, Available: true}
	if err := service.finishSuccess(task, run, subagentport.AgentRunResult{Content: "done", Usage: usage}, domainworkspace.ChangeSet{}); err != nil {
		t.Fatal(err)
	}
	current, err := repository.GetTask(context.Background(), task.ID)
	if err != nil || current.Usage.TotalTokens != 20 {
		t.Fatalf("task did not accumulate child usage: %#v, %v", current.Usage, err)
	}
	runs, err := repository.ListRuns(context.Background(), task.ID)
	if err != nil || len(runs) != 1 || runs[0].Usage != usage {
		t.Fatalf("turn usage was not persisted: %#v, %v", runs, err)
	}
}

func TestLosingLeaseCancelsExecutionWithoutTerminalWrite(t *testing.T) {
	repository := memory.NewRepository()
	task := domainsubagent.Task{ID: "task", ParentSessionID: "parent", ChildSessionID: "child",
		CurrentRunID: "run", Status: domainsubagent.TaskStatusQueued,
		Definition: domainsubagent.DefinitionSnapshot{TimeoutSeconds: 5},
		Workspace:  domainworkspace.Reference{Mode: domainworkspace.IsolationModeDirect}}
	run := domainsubagent.Run{ID: "run", TaskID: task.ID, Status: domainsubagent.RunStatusQueued}
	if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
		t.Fatal(err)
	}
	service := &Service{Tasks: repository, Runs: repository, TaskRuns: repository,
		ExecutionLease: repository, ExecutionOwnerID: "old-instance", ExecutionLeaseTTL: 900 * time.Millisecond,
		Sessions: &fakeChildSessions{}, Runner: leaseBlockingRunner{started: make(chan struct{})}}
	if err := service.acquireExecutionLease(context.Background(), task.ID, run.ID); err != nil {
		t.Fatal(err)
	}
	if !service.registerActiveRun(task.ID, run.ID) {
		t.Fatal("failed to register execution")
	}
	done := make(chan struct{})
	go func() {
		service.execute(context.Background(), task.ID, run.ID, "model")
		close(done)
	}()
	select {
	case <-service.Runner.(leaseBlockingRunner).started:
	case <-time.After(3 * time.Second):
		t.Fatal("runner did not start")
	}
	if err := repository.Release(context.Background(), task.ID, run.ID, "old-instance"); err != nil {
		t.Fatal(err)
	}
	if err := repository.Acquire(context.Background(), task.ID, run.ID, "new-instance", time.Minute); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("old execution did not stop after losing lease")
	}
	current, err := repository.GetTask(context.Background(), task.ID)
	if err != nil || current.Status != domainsubagent.TaskStatusRunning {
		t.Fatalf("old execution committed terminal state after lease transfer: %#v, %v", current, err)
	}
}

func TestCancellationDoesNotRequireExecutionLeaseOwner(t *testing.T) {
	repository := memory.NewRepository()
	task := domainsubagent.Task{ID: "task", ParentSessionID: "parent", CurrentRunID: "run", Status: domainsubagent.TaskStatusRunning}
	run := domainsubagent.Run{ID: "run", TaskID: task.ID, Status: domainsubagent.RunStatusRunning}
	if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
		t.Fatal(err)
	}
	if err := repository.Acquire(context.Background(), task.ID, run.ID, "worker-a", time.Minute); err != nil {
		t.Fatal(err)
	}
	service := &Service{Tasks: repository, Runs: repository, Scheduler: &fakeScheduler{},
		ExecutionLease: repository, ExecutionOwnerID: "api-b"}
	result, err := service.Cancel(context.Background(), subagentcommand.CancelTask{TaskID: task.ID, Reason: "canceled remotely"})
	if err != nil {
		t.Fatalf("cross-instance cancellation failed: %v", err)
	}
	if result.Value.Status != domainsubagent.TaskStatusCanceled {
		t.Fatalf("expected canceled result, got %s", result.Value.Status)
	}
	current, err := repository.GetTask(context.Background(), task.ID)
	if err != nil || current.Status != domainsubagent.TaskStatusCanceled {
		t.Fatalf("persisted cancellation missing: %#v, %v", current, err)
	}
}

func TestTaskOnlyMutationsRespectExecutionLease(t *testing.T) {
	repository := memory.NewRepository()
	task := domainsubagent.Task{ID: "task", ParentSessionID: "parent", CurrentRunID: "run", Status: domainsubagent.TaskStatusSucceeded,
		Workspace: domainworkspace.Reference{ID: "workspace", Mode: domainworkspace.IsolationModeSnapshot, Root: "snapshot"}, Unread: true, Result: "done"}
	run := domainsubagent.Run{ID: "run", TaskID: task.ID, Status: domainsubagent.RunStatusSucceeded}
	if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
		t.Fatal(err)
	}
	if err := repository.Acquire(context.Background(), task.ID, "other-operation", "worker-a", time.Minute); err != nil {
		t.Fatal(err)
	}
	service := &Service{Tasks: repository, Runs: repository, TaskRuns: repository, ExecutionLease: repository,
		ExecutionOwnerID: "api-b", Workspaces: &fakeWorkspaceManager{}, ParentContinuation: &fakeParentContinuation{}}
	if _, err := service.ApplyChanges(context.Background(), subagentcommand.ApplyTaskChanges{TaskID: task.ID, ParentSessionID: task.ParentSessionID}); !errors.Is(err, subagentport.ErrExecutionLeaseNotAcquired) {
		t.Fatalf("apply ignored another operation lease: %v", err)
	}
	if _, err := service.Resume(context.Background(), subagentcommand.ResumeTask{TaskID: task.ID, ParentSessionID: task.ParentSessionID}); !errors.Is(err, subagentport.ErrExecutionLeaseNotAcquired) {
		t.Fatalf("resume ignored another operation lease: %v", err)
	}
}

type leaseBlockingRunner struct{ started chan struct{} }

func (runner leaseBlockingRunner) Run(ctx context.Context, _ subagentport.AgentRunRequest) (subagentport.AgentRunResult, error) {
	close(runner.started)
	<-ctx.Done()
	return subagentport.AgentRunResult{}, ctx.Err()
}
