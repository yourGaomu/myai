package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"myai/core/adapter/subagent/memory"
	subagentcommand "myai/core/application/subagent/command"
	domainsubagent "myai/core/domain/subagent"
	domainworkspace "myai/core/domain/workspace"
	subagentport "myai/core/port/subagent"
	workspaceport "myai/core/port/workspace"
	"myai/core/session"
)

func TestTaskUsesPreparedSnapshotAndCollectsChanges(t *testing.T) {
	repository := memory.NewRepository()
	registry := memory.NewRegistry()
	definition := writableDefinition()
	if err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	scheduler := &fakeScheduler{}
	workspaces := &fakeWorkspaceManager{}
	sessions := &fakeChildSessions{}
	service := &Service{
		Definitions: repository, Registry: registry, Tasks: repository, Runs: repository,
		Scheduler: scheduler, Sessions: sessions, Runner: fakeRunner{}, IDs: &sequenceIDs{},
		Workspaces: workspaces, DefaultWorkspaceRoot: "C:/workspace",
	}

	started, err := service.Start(context.Background(), subagentcommand.StartTask{
		ParentSessionID: "parent-1", CreatedRequestID: "request-1", DefinitionID: definition.ID,
		Instruction: "Implement the change", FallbackModelID: "model-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if started.Value.Status != domainsubagent.TaskStatusQueued || scheduler.task == nil {
		t.Fatalf("unexpected queued task: %#v", started.Value)
	}
	scheduler.task(context.Background())

	completed, err := repository.GetTask(context.Background(), started.Value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != domainsubagent.TaskStatusSucceeded {
		t.Fatalf("expected succeeded task, got %#v", completed)
	}
	if completed.Workspace.Root != "C:/snapshots/task" || sessions.request.WorkspaceRoot != "C:/snapshots/task" {
		t.Fatalf("expected child session to use prepared root, task=%#v request=%#v", completed.Workspace, sessions.request)
	}
	if completed.ChangeSet.Status != domainworkspace.ChangeSetStatusPending || len(completed.ChangeSet.Files) != 1 {
		t.Fatalf("unexpected collected changes: %#v", completed.ChangeSet)
	}
}

func TestCheckRejectsPollingFromCreatingRequest(t *testing.T) {
	repository := memory.NewRepository()
	task := domainsubagent.Task{ID: "task-1", ParentSessionID: "parent-1", CreatedRequestID: "request-1", Status: domainsubagent.TaskStatusRunning}
	if err := repository.SaveTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	service := &Service{Tasks: repository}
	_, err := service.Check(context.Background(), subagentcommand.CheckTask{
		TaskID: task.ID, ParentSessionID: task.ParentSessionID, RequestID: task.CreatedRequestID,
	})
	if err == nil {
		t.Fatal("expected same-request polling to be rejected")
	}
}

func TestTaskFailsWhenSubagentReturnsEmptyResult(t *testing.T) {
	repository := memory.NewRepository()
	registry := memory.NewRegistry()
	definition := writableDefinition()
	if err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	scheduler := &fakeScheduler{}
	service := &Service{
		Definitions: repository, Registry: registry, Tasks: repository, Runs: repository,
		Scheduler: scheduler, Sessions: &fakeChildSessions{}, Runner: emptyRunner{}, IDs: &sequenceIDs{},
		Workspaces: &fakeWorkspaceManager{}, DefaultWorkspaceRoot: "C:/workspace",
	}

	started, err := service.Start(context.Background(), subagentcommand.StartTask{
		ParentSessionID: "parent-1", CreatedRequestID: "request-1", DefinitionID: definition.ID,
		Instruction: "Inspect the workspace", FallbackModelID: "model-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	scheduler.task(context.Background())

	completed, err := repository.GetTask(context.Background(), started.Value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != domainsubagent.TaskStatusFailed || completed.ErrorMessage != "subagent returned an empty result" {
		t.Fatalf("expected empty result failure, got %#v", completed)
	}
}

func TestApplyPersistsConflictChangeSet(t *testing.T) {
	repository := memory.NewRepository()
	task := domainsubagent.Task{
		ID: "task-1", ParentSessionID: "parent-1", Status: domainsubagent.TaskStatusSucceeded,
		Workspace: domainworkspace.Reference{ID: "workspace-1", Mode: domainworkspace.IsolationModeSnapshot, Root: "snapshot"},
		ChangeSet: domainworkspace.ChangeSet{WorkspaceID: "workspace-1", Status: domainworkspace.ChangeSetStatusPending},
	}
	if err := repository.SaveTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	workspaces := &fakeWorkspaceManager{conflict: true}
	service := &Service{Tasks: repository, Workspaces: workspaces}
	result, err := service.ApplyChanges(context.Background(), subagentcommand.ApplyTaskChanges{
		TaskID: task.ID, ParentSessionID: task.ParentSessionID,
	})
	if !errors.Is(err, workspaceport.ErrConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
	if result.Value.ChangeSet.Status != domainworkspace.ChangeSetStatusConflict {
		t.Fatalf("expected persisted conflict status, got %#v", result.Value.ChangeSet)
	}
}

func TestScheduleFailureMarksTaskAndRunFailed(t *testing.T) {
	repository := memory.NewRepository()
	registry := memory.NewRegistry()
	definition := writableDefinition()
	if err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	service := &Service{
		Definitions: repository, Registry: registry, Tasks: repository, Runs: repository,
		Scheduler: failingScheduler{}, Sessions: &fakeChildSessions{}, Runner: fakeRunner{}, IDs: &sequenceIDs{},
		Workspaces: &fakeWorkspaceManager{}, DefaultWorkspaceRoot: "C:/workspace",
	}

	result, err := service.Start(context.Background(), subagentcommand.StartTask{
		ParentSessionID: "parent-1", DefinitionID: definition.ID, Instruction: "Inspect the workspace",
	})
	if err == nil || result.Value.Status != domainsubagent.TaskStatusFailed {
		t.Fatalf("expected failed scheduling result, got result=%#v err=%v", result.Value, err)
	}
	runs, listErr := repository.ListRuns(context.Background(), result.Value.ID)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(runs) != 1 || runs[0].Status != domainsubagent.RunStatusFailed || runs[0].ErrorMessage == "" {
		t.Fatalf("expected failed run, got %#v", runs)
	}
}

func TestExecutionPanicMarksTaskAndRunFailed(t *testing.T) {
	repository := memory.NewRepository()
	registry := memory.NewRegistry()
	definition := writableDefinition()
	if err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	scheduler := &fakeScheduler{}
	var reported error
	service := &Service{
		Definitions: repository, Registry: registry, Tasks: repository, Runs: repository,
		Scheduler: scheduler, Sessions: &fakeChildSessions{}, Runner: panicRunner{}, IDs: &sequenceIDs{},
		Workspaces: &fakeWorkspaceManager{}, DefaultWorkspaceRoot: "C:/workspace",
		OnError: func(err error) { reported = err },
	}

	started, err := service.Start(context.Background(), subagentcommand.StartTask{
		ParentSessionID: "parent-1", DefinitionID: definition.ID, Instruction: "Inspect the workspace",
	})
	if err != nil {
		t.Fatal(err)
	}
	scheduler.task(context.Background())
	completed, err := repository.GetTask(context.Background(), started.Value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != domainsubagent.TaskStatusFailed || !strings.Contains(completed.ErrorMessage, "panicked") {
		t.Fatalf("expected panic failure, got %#v", completed)
	}
	if reported == nil || !strings.Contains(reported.Error(), "runner panic") {
		t.Fatalf("expected reported panic, got %v", reported)
	}
	runs, err := repository.ListRuns(context.Background(), completed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != domainsubagent.RunStatusFailed {
		t.Fatalf("expected failed run, got %#v", runs)
	}
}

func TestRecoverInterruptedTasksMarksTaskAndRunFailed(t *testing.T) {
	repository := memory.NewRepository()
	createdAt := time.Date(2026, 7, 25, 8, 0, 0, 0, time.UTC)
	task := domainsubagent.Task{
		ID: "task-1", ParentSessionID: "parent-1", DefinitionID: "coder", Instruction: "Continue work",
		Status: domainsubagent.TaskStatusRunning, CurrentRunID: "run-1", CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	run := domainsubagent.Run{
		ID: "run-1", TaskID: task.ID, Sequence: 1, Instruction: task.Instruction,
		Status: domainsubagent.RunStatusRunning, CreatedAt: createdAt,
	}
	if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
		t.Fatal(err)
	}
	service := &Service{Tasks: repository, Runs: repository, TaskRuns: repository, Now: func() time.Time {
		return createdAt.Add(time.Minute)
	}}
	if err := service.RecoverInterruptedTasks(context.Background()); err != nil {
		t.Fatal(err)
	}
	recovered, err := repository.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != domainsubagent.TaskStatusFailed || recovered.ErrorMessage != interruptedTaskMessage {
		t.Fatalf("unexpected recovered task: %#v", recovered)
	}
	runs, err := repository.ListRuns(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != domainsubagent.RunStatusFailed || runs[0].ErrorMessage != interruptedTaskMessage {
		t.Fatalf("unexpected recovered runs: %#v", runs)
	}
}

func TestResumeClaimsUnreadTaskAndContinuesParent(t *testing.T) {
	repository := memory.NewRepository()
	task := resumableTask()
	if err := repository.SaveTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	continuation := &fakeParentContinuation{}
	service := &Service{Tasks: repository, ParentContinuation: continuation}

	result, err := service.Resume(context.Background(), subagentcommand.ResumeTask{
		TaskID: task.ID, ParentSessionID: task.ParentSessionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if continuation.request.Task.ID != task.ID || continuation.request.Task.Unread {
		t.Fatalf("unexpected continuation request: %#v", continuation.request)
	}
	if result.Content != "parent continued" || result.Task.Unread {
		t.Fatalf("unexpected resume result: %#v", result)
	}
	persisted, err := repository.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Unread {
		t.Fatalf("expected result to be consumed: %#v", persisted)
	}
}

func TestResumeRestoresUnreadWhenParentContinuationFails(t *testing.T) {
	repository := memory.NewRepository()
	task := resumableTask()
	if err := repository.SaveTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	service := &Service{
		Tasks: repository, ParentContinuation: &fakeParentContinuation{err: errors.New("model unavailable")},
	}

	result, err := service.Resume(context.Background(), subagentcommand.ResumeTask{
		TaskID: task.ID, ParentSessionID: task.ParentSessionID,
	})
	if err == nil || result.Task.ID != task.ID || !result.Task.Unread {
		t.Fatalf("expected retryable failure, result=%#v err=%v", result, err)
	}
	persisted, getErr := repository.GetTask(context.Background(), task.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if !persisted.Unread {
		t.Fatalf("expected unread result to be restored: %#v", persisted)
	}
}

func TestResumeRejectsInvalidTaskStateParentAndConsumedResult(t *testing.T) {
	tests := []struct {
		name     string
		task     domainsubagent.Task
		parentID string
	}{
		{name: "running", task: domainsubagent.Task{ID: "task-running", ParentSessionID: "parent-1", Status: domainsubagent.TaskStatusRunning, Unread: true}, parentID: "parent-1"},
		{name: "wrong parent", task: resumableTask(), parentID: "parent-2"},
		{name: "consumed", task: func() domainsubagent.Task { value := resumableTask(); value.Unread = false; return value }(), parentID: "parent-1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := memory.NewRepository()
			if err := repository.SaveTask(context.Background(), test.task); err != nil {
				t.Fatal(err)
			}
			continuation := &fakeParentContinuation{}
			_, err := (&Service{Tasks: repository, ParentContinuation: continuation}).Resume(context.Background(), subagentcommand.ResumeTask{
				TaskID: test.task.ID, ParentSessionID: test.parentID,
			})
			if err == nil || continuation.called {
				t.Fatalf("expected resume rejection before continuation, err=%v called=%v", err, continuation.called)
			}
		})
	}
}

func resumableTask() domainsubagent.Task {
	return domainsubagent.Task{
		ID: "task-1", ParentSessionID: "parent-1", Status: domainsubagent.TaskStatusSucceeded,
		Title: "Inspect implementation", Result: "inspection complete", Unread: true,
	}
}

func writableDefinition() domainsubagent.Definition {
	return domainsubagent.Definition{
		ID: "coder", Name: "Coder", SystemPrompt: "Implement changes in the isolated workspace.",
		AllowedTools: []string{"read_file", "edit_file"}, CapabilityMode: domainsubagent.CapabilityModeReadWrite,
		IsolationMode: domainworkspace.IsolationModeSnapshot, MaxTurns: 4, TimeoutSeconds: 60,
		Enabled: true, Version: 1, Source: domainsubagent.DefinitionSourceUser,
	}
}

type sequenceIDs struct{ next int }

func (ids *sequenceIDs) NewID() string {
	ids.next++
	return "id-" + string(rune('0'+ids.next))
}

type fakeScheduler struct{ task subagentport.ScheduledTask }

func (scheduler *fakeScheduler) Submit(_ string, task subagentport.ScheduledTask) error {
	scheduler.task = task
	return nil
}
func (*fakeScheduler) Cancel(string) bool { return true }

type failingScheduler struct{}

func (failingScheduler) Submit(string, subagentport.ScheduledTask) error {
	return errors.New("queue unavailable")
}
func (failingScheduler) Cancel(string) bool { return false }

type fakeChildSessions struct {
	request subagentport.ChildSessionRequest
}

func (sessions *fakeChildSessions) Create(_ context.Context, request subagentport.ChildSessionRequest) (*session.Session, error) {
	sessions.request = request
	return session.NewFromState(session.InitialState{ID: request.SessionID, Model: request.FallbackModelID}), nil
}

type fakeRunner struct{}

func (fakeRunner) Run(context.Context, subagentport.AgentRunRequest) (subagentport.AgentRunResult, error) {
	return subagentport.AgentRunResult{Content: "done"}, nil
}

type emptyRunner struct{}

func (emptyRunner) Run(context.Context, subagentport.AgentRunRequest) (subagentport.AgentRunResult, error) {
	return subagentport.AgentRunResult{}, nil
}

type panicRunner struct{}

func (panicRunner) Run(context.Context, subagentport.AgentRunRequest) (subagentport.AgentRunResult, error) {
	panic("runner panic")
}

type fakeParentContinuation struct {
	called  bool
	request subagentport.ParentContinuationRequest
	err     error
}

func (continuation *fakeParentContinuation) Continue(_ context.Context, request subagentport.ParentContinuationRequest) (subagentport.ParentContinuationResult, error) {
	continuation.called = true
	continuation.request = request
	if continuation.err != nil {
		return subagentport.ParentContinuationResult{}, continuation.err
	}
	return subagentport.ParentContinuationResult{Content: "parent continued"}, nil
}

type fakeWorkspaceManager struct{ conflict bool }

func (*fakeWorkspaceManager) Prepare(_ context.Context, request workspaceport.PrepareRequest) (workspaceport.PreparedWorkspace, error) {
	return workspaceport.PreparedWorkspace{Reference: domainworkspace.Reference{
		ID: request.WorkspaceID, Mode: domainworkspace.IsolationModeSnapshot, Root: "C:/snapshots/task",
	}}, nil
}
func (*fakeWorkspaceManager) Collect(_ context.Context, request workspaceport.CollectRequest) (workspaceport.CollectedChanges, error) {
	return workspaceport.CollectedChanges{ChangeSet: domainworkspace.ChangeSet{
		WorkspaceID: request.Reference.ID, Status: domainworkspace.ChangeSetStatusPending,
		Files: []domainworkspace.FileChange{{Path: "main.go", ChangeType: domainworkspace.FileChangeModified}},
	}}, nil
}
func (manager *fakeWorkspaceManager) Apply(_ context.Context, request workspaceport.ApplyRequest) (workspaceport.AppliedChanges, error) {
	if manager.conflict {
		return workspaceport.AppliedChanges{ChangeSet: domainworkspace.ChangeSet{
			WorkspaceID: request.Reference.ID, Status: domainworkspace.ChangeSetStatusConflict,
		}}, workspaceport.ErrConflict
	}
	return workspaceport.AppliedChanges{ChangeSet: domainworkspace.ChangeSet{
		WorkspaceID: request.Reference.ID, Status: domainworkspace.ChangeSetStatusApplied,
	}}, nil
}
func (*fakeWorkspaceManager) Discard(_ context.Context, request workspaceport.DiscardRequest) (workspaceport.DiscardedChanges, error) {
	return workspaceport.DiscardedChanges{ChangeSet: domainworkspace.ChangeSet{
		WorkspaceID: request.Reference.ID, Status: domainworkspace.ChangeSetStatusDiscarded,
	}}, nil
}
