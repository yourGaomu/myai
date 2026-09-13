package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	subagentevents "myai/core/adapter/subagent/events"
	"myai/core/adapter/subagent/memory"
	subagentcommand "myai/core/application/subagent/command"
	subagentresult "myai/core/application/subagent/result"
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

func TestSendMessageDeliversMailboxBetweenChildTurns(t *testing.T) {
	repository := memory.NewRepository()
	registry := memory.NewRegistry()
	definition := writableDefinition()
	if err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	scheduler := &fakeScheduler{}
	runner := &recordingRunner{}
	service := &Service{
		Definitions: repository, Registry: registry, Tasks: repository, Runs: repository,
		Scheduler: scheduler, Sessions: &fakeChildSessions{}, Runner: runner, IDs: &sequenceIDs{},
		Workspaces: &fakeWorkspaceManager{}, DefaultWorkspaceRoot: "C:/workspace",
	}
	started, err := service.Start(context.Background(), subagentcommand.StartTask{
		ParentSessionID: "parent-1", DefinitionID: definition.ID, Instruction: "inspect", FallbackModelID: "model-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SendMessage(context.Background(), subagentcommand.SendMessage{
		TaskID: started.Value.ID, ParentSessionID: "parent-1", Content: "also check the tests",
	}); err != nil {
		t.Fatal(err)
	}
	scheduler.task(context.Background())
	if len(runner.instructions) != 2 || runner.instructions[1] != "also check the tests" {
		t.Fatalf("expected initial and mailbox turns, got %#v", runner.instructions)
	}
	task, err := repository.GetTask(context.Background(), started.Value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(task.Mailbox) != 0 || !strings.Contains(task.Result, "follow-up result") {
		t.Fatalf("mailbox was not drained into result: %#v", task)
	}
}

func TestFollowupStartsNewRunAndReusesChildSession(t *testing.T) {
	repository := memory.NewRepository()
	registry := memory.NewRegistry()
	definition := writableDefinition()
	if err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	scheduler := &fakeScheduler{}
	sessions := &fakeChildSessions{}
	runner := &recordingRunner{}
	service := &Service{
		Definitions: repository, Registry: registry, Tasks: repository, Runs: repository,
		Scheduler: scheduler, Sessions: sessions, Runner: runner, IDs: &sequenceIDs{},
		Workspaces: &fakeWorkspaceManager{}, DefaultWorkspaceRoot: "C:/workspace",
	}
	started, err := service.Start(context.Background(), subagentcommand.StartTask{
		ParentSessionID: "parent-1", DefinitionID: definition.ID, Instruction: "inspect", FallbackModelID: "model-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	firstRunID := started.Value.CurrentRunID
	scheduler.task(context.Background())
	completed, err := repository.GetTask(context.Background(), started.Value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != domainsubagent.TaskStatusSucceeded || !completed.Unread {
		t.Fatalf("initial child run did not complete: %#v", completed)
	}
	followup, err := service.Followup(context.Background(), subagentcommand.FollowupTask{
		TaskID: started.Value.ID, ParentSessionID: "parent-1", Content: "now inspect the tests too",
	})
	if err != nil {
		t.Fatal(err)
	}
	if followup.Value.Status != domainsubagent.TaskStatusQueued || followup.Value.CurrentRunID == firstRunID || followup.Value.Unread {
		t.Fatalf("follow-up did not queue a fresh run: %#v", followup.Value)
	}
	runs, err := repository.ListRuns(context.Background(), started.Value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[1].Sequence != 2 || runs[1].Instruction != "now inspect the tests too" {
		t.Fatalf("expected preserved first run and second follow-up run: %#v", runs)
	}
	scheduler.task(context.Background())
	final, err := repository.GetTask(context.Background(), started.Value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != domainsubagent.TaskStatusSucceeded || len(runner.instructions) != 2 || runner.instructions[1] != "now inspect the tests too" {
		t.Fatalf("follow-up execution did not reuse child session: task=%#v instructions=%#v sessions=%d", final, runner.instructions, sessions.creates)
	}
	if sessions.creates != 2 {
		t.Fatalf("expected child session factory to be called for each run, got %d", sessions.creates)
	}
}

func TestFollowupRejectsResultBeingConsumed(t *testing.T) {
	repository := memory.NewRepository()
	task := domainsubagent.Task{
		ID: "completed", ParentSessionID: "parent-1", Status: domainsubagent.TaskStatusSucceeded,
		CurrentRunID: "run-1", CreatedAt: time.Now().UTC(), Unread: true,
	}
	run := domainsubagent.Run{ID: "run-1", TaskID: task.ID, Sequence: 1, Status: domainsubagent.RunStatusSucceeded, CreatedAt: task.CreatedAt}
	if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
		t.Fatal(err)
	}
	service := &Service{
		Tasks: repository, Runs: repository, TaskRuns: repository, Scheduler: &fakeScheduler{}, IDs: &sequenceIDs{},
		resumeClaims: map[string]struct{}{task.ID: {}},
	}
	if _, err := service.Followup(context.Background(), subagentcommand.FollowupTask{
		TaskID: task.ID, ParentSessionID: task.ParentSessionID, Content: "continue",
	}); err == nil || !strings.Contains(err.Error(), "being consumed") {
		t.Fatalf("expected resume claim to block follow-up, got %v", err)
	}
}

func TestMailboxFailureKeepsFailedAndLaterMessagesPending(t *testing.T) {
	repository := memory.NewRepository()
	registry := memory.NewRegistry()
	definition := writableDefinition()
	if err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	scheduler := &fakeScheduler{}
	runner := &failingFollowUpRunner{}
	service := &Service{
		Definitions: repository, Registry: registry, Tasks: repository, Runs: repository,
		Scheduler: scheduler, Sessions: &fakeChildSessions{}, Runner: runner, IDs: &sequenceIDs{},
		Workspaces: &fakeWorkspaceManager{}, DefaultWorkspaceRoot: "C:/workspace",
	}
	started, err := service.Start(context.Background(), subagentcommand.StartTask{
		ParentSessionID: "parent-1", DefinitionID: definition.ID, Instruction: "inspect", FallbackModelID: "model-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"first follow-up", "second follow-up"} {
		if _, err := service.SendMessage(context.Background(), subagentcommand.SendMessage{
			TaskID: started.Value.ID, ParentSessionID: "parent-1", Content: content,
		}); err != nil {
			t.Fatal(err)
		}
	}
	scheduler.task(context.Background())

	task, err := repository.GetTask(context.Background(), started.Value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != domainsubagent.TaskStatusFailed || len(task.Mailbox) != 2 {
		t.Fatalf("failed delivery lost mailbox data: %#v", task)
	}
	if task.Mailbox[0].Status != domainsubagent.MessageStatusPending || task.Mailbox[0].DeliveryAttempts != 1 || task.Mailbox[0].LastError == "" {
		t.Fatalf("failed message was not released for retry: %#v", task.Mailbox[0])
	}
	if task.Mailbox[1].Status != domainsubagent.MessageStatusPending || task.Mailbox[1].DeliveryAttempts != 0 {
		t.Fatalf("later message was claimed or lost: %#v", task.Mailbox[1])
	}
}

func TestMailboxPanicReleasesClaimedMessage(t *testing.T) {
	repository := memory.NewRepository()
	registry := memory.NewRegistry()
	definition := writableDefinition()
	if err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	scheduler := &fakeScheduler{}
	runner := &panicFollowUpRunner{}
	service := &Service{
		Definitions: repository, Registry: registry, Tasks: repository, Runs: repository,
		Scheduler: scheduler, Sessions: &fakeChildSessions{}, Runner: runner, IDs: &sequenceIDs{},
		Workspaces: &fakeWorkspaceManager{}, DefaultWorkspaceRoot: "C:/workspace",
	}
	started, err := service.Start(context.Background(), subagentcommand.StartTask{
		ParentSessionID: "parent-1", DefinitionID: definition.ID, Instruction: "inspect", FallbackModelID: "model-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SendMessage(context.Background(), subagentcommand.SendMessage{
		TaskID: started.Value.ID, ParentSessionID: "parent-1", Content: "follow up",
	}); err != nil {
		t.Fatal(err)
	}
	scheduler.task(context.Background())

	task, err := repository.GetTask(context.Background(), started.Value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != domainsubagent.TaskStatusFailed || len(task.Mailbox) != 1 {
		t.Fatalf("panic lost mailbox state: %#v", task)
	}
	if task.Mailbox[0].Status != domainsubagent.MessageStatusPending || task.Mailbox[0].DeliveryAttempts != 1 || task.Mailbox[0].ClaimedAt != nil {
		t.Fatalf("panicked delivery was not released: %#v", task.Mailbox[0])
	}
}

func TestListIncludesNestedTaskTree(t *testing.T) {
	repository := memory.NewRepository()
	root := domainsubagent.Task{ID: "root", ParentSessionID: "session-1", ChildSessionID: "child-session", Title: "root", Status: domainsubagent.TaskStatusRunning}
	child := domainsubagent.Task{ID: "child", ParentSessionID: "child-session", ParentTaskID: "root", ChildSessionID: "grandchild-session", Title: "child", Status: domainsubagent.TaskStatusSucceeded}
	grandchild := domainsubagent.Task{ID: "grandchild", ParentSessionID: "grandchild-session", ParentTaskID: "child", Title: "grandchild", Status: domainsubagent.TaskStatusQueued}
	for _, task := range []domainsubagent.Task{root, child, grandchild} {
		if err := repository.SaveTask(context.Background(), task); err != nil {
			t.Fatal(err)
		}
	}
	result, err := (&Service{Tasks: repository}).List(context.Background(), subagentcommand.ListTasks{ParentSessionID: "session-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("expected complete nested task tree, got %d items", len(result.Items))
	}
}

func TestNestedTaskRecordsParentAndHierarchicalAgentPath(t *testing.T) {
	repository := memory.NewRepository()
	registry := memory.NewRegistry()
	definition := writableDefinition()
	if err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	parent := domainsubagent.Task{
		ID: "parent-task", ParentSessionID: "root-session", ChildSessionID: "child-session",
		AgentPath: "review/scan", DefinitionID: definition.ID, Instruction: "parent", Status: domainsubagent.TaskStatusRunning,
	}
	if err := repository.SaveTask(context.Background(), parent); err != nil {
		t.Fatal(err)
	}
	service := &Service{
		Definitions: repository, Registry: registry, Tasks: repository, Runs: repository,
		Scheduler: &fakeScheduler{}, Sessions: &fakeChildSessions{}, Runner: fakeRunner{}, IDs: &sequenceIDs{},
		Workspaces: &fakeWorkspaceManager{}, DefaultWorkspaceRoot: "C:/workspace",
	}
	result, err := service.Start(context.Background(), subagentcommand.StartTask{
		ParentSessionID: "child-session", ParentTaskID: parent.ID, DefinitionID: definition.ID,
		Instruction: "nested", TaskName: "verify", FallbackModelID: "model-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Value.ParentTaskID != parent.ID || result.Value.AgentPath != "review/scan/verify" {
		t.Fatalf("unexpected nested identity: %#v", result.Value)
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

func TestWaitBlocksUntilTerminalTaskEvent(t *testing.T) {
	repository := memory.NewRepository()
	bus := subagentevents.NewBus()
	task := domainsubagent.Task{ID: "task-1", ParentSessionID: "parent-1", Status: domainsubagent.TaskStatusRunning}
	if err := repository.SaveTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	service := &Service{Tasks: repository, Events: bus}
	resultCh := make(chan struct {
		result subagentresult.Wait
		err    error
	}, 1)
	go func() {
		result, err := service.Wait(context.Background(), subagentcommand.WaitTask{
			TaskID: task.ID, ParentSessionID: task.ParentSessionID, Timeout: time.Second,
		})
		resultCh <- struct {
			result subagentresult.Wait
			err    error
		}{result: result, err: err}
	}()
	select {
	case result := <-resultCh:
		t.Fatalf("wait returned before terminal event: %#v", result)
	case <-time.After(50 * time.Millisecond):
	}

	if err := task.MarkSucceeded("done", "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	bus.TaskUpdated(context.Background(), task)

	select {
	case result := <-resultCh:
		if result.err != nil || result.result.Task.Status != domainsubagent.TaskStatusSucceeded || result.result.Sequence == 0 {
			t.Fatalf("unexpected wait result: %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("wait did not wake after terminal event")
	}
}

func TestWaitMarksNestedParentAsWaitingAndRestoresIt(t *testing.T) {
	repository := memory.NewRepository()
	bus := subagentevents.NewBus()
	parent := domainsubagent.Task{ID: "parent-task", ParentSessionID: "root-session", ChildSessionID: "parent-session", Status: domainsubagent.TaskStatusRunning}
	child := domainsubagent.Task{ID: "child-task", ParentSessionID: "parent-session", ParentTaskID: parent.ID, Status: domainsubagent.TaskStatusRunning}
	if err := repository.SaveTask(context.Background(), parent); err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveTask(context.Background(), child); err != nil {
		t.Fatal(err)
	}
	service := &Service{Tasks: repository, Events: bus}
	resultCh := make(chan error, 1)
	go func() {
		_, err := service.Wait(context.Background(), subagentcommand.WaitTask{
			TaskID: child.ID, ParentSessionID: child.ParentSessionID, ParentTaskID: parent.ID, Timeout: time.Second,
		})
		resultCh <- err
	}()
	deadline := time.After(time.Second)
	for {
		current, err := repository.GetTask(context.Background(), parent.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status == domainsubagent.TaskStatusWaitingSubagents {
			break
		}
		select {
		case <-deadline:
			t.Fatal("parent task did not enter waiting_subagents")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	if err := child.MarkSucceeded("done", "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveTask(context.Background(), child); err != nil {
		t.Fatal(err)
	}
	bus.TaskUpdated(context.Background(), child)
	select {
	case err := <-resultCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("wait did not complete")
	}
	current, err := repository.GetTask(context.Background(), parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domainsubagent.TaskStatusRunning {
		t.Fatalf("expected parent to resume running, got %s", current.Status)
	}
}

func TestWaitingChildIsWokenByMailboxMessage(t *testing.T) {
	repository := memory.NewRepository()
	bus := subagentevents.NewBus()
	waitingChild := domainsubagent.Task{
		ID: "waiting-child", ParentSessionID: "root-session", ChildSessionID: "child-session",
		Status: domainsubagent.TaskStatusRunning,
	}
	grandchild := domainsubagent.Task{
		ID: "grandchild", ParentSessionID: "child-session", ParentTaskID: waitingChild.ID,
		Status: domainsubagent.TaskStatusRunning,
	}
	for _, task := range []domainsubagent.Task{waitingChild, grandchild} {
		if err := repository.SaveTask(context.Background(), task); err != nil {
			t.Fatal(err)
		}
	}
	service := &Service{Tasks: repository, Events: bus, IDs: &sequenceIDs{}}
	resultCh := make(chan struct {
		result subagentresult.Wait
		err    error
	}, 1)
	go func() {
		result, err := service.Wait(context.Background(), subagentcommand.WaitTask{
			TaskID: grandchild.ID, ParentSessionID: waitingChild.ChildSessionID,
			ParentTaskID: waitingChild.ID, Timeout: time.Second,
		})
		resultCh <- struct {
			result subagentresult.Wait
			err    error
		}{result: result, err: err}
	}()
	deadline := time.After(time.Second)
	for {
		current, err := repository.GetTask(context.Background(), waitingChild.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status == domainsubagent.TaskStatusWaitingSubagents {
			break
		}
		select {
		case <-deadline:
			t.Fatal("waiting child did not enter waiting_subagents")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	if _, err := service.SendMessage(context.Background(), subagentcommand.SendMessage{
		TaskID: waitingChild.ID, ParentSessionID: waitingChild.ParentSessionID, Content: "handle this new requirement",
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case outcome := <-resultCh:
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		if !outcome.result.WokenByMailbox || outcome.result.Task.ID != grandchild.ID {
			t.Fatalf("unexpected mailbox wake result: %#v", outcome.result)
		}
	case <-time.After(time.Second):
		t.Fatal("mailbox message did not wake waiting child")
	}
	current, err := repository.GetTask(context.Background(), waitingChild.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domainsubagent.TaskStatusRunning || len(current.Mailbox) != 1 {
		t.Fatalf("expected waiting child to resume with a pending mailbox message: %#v", current)
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

func TestRecoverInterruptedTasksRequeuesRunningTaskWhenSchedulerIsAvailable(t *testing.T) {
	repository := memory.NewRepository()
	createdAt := time.Date(2026, 7, 25, 8, 0, 0, 0, time.UTC)
	claimedAt := createdAt.Add(time.Second)
	task := domainsubagent.Task{
		ID: "task-requeue", ParentSessionID: "parent-1", DefinitionID: "coder", Instruction: "Continue work",
		Definition: domainsubagent.DefinitionSnapshot{ModelID: "model-1"},
		Status:     domainsubagent.TaskStatusRunning, CurrentRunID: "run-requeue", CreatedAt: createdAt, UpdatedAt: createdAt,
		Mailbox: []domainsubagent.Message{{
			ID: "message-1", Content: "retry after restart", Status: domainsubagent.MessageStatusDelivering,
			DeliveryAttempts: 1, CreatedAt: createdAt, ClaimedAt: &claimedAt,
		}},
	}
	run := domainsubagent.Run{
		ID: "run-requeue", TaskID: task.ID, Sequence: 1, Instruction: task.Instruction,
		Status: domainsubagent.RunStatusRunning, CreatedAt: createdAt,
	}
	if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
		t.Fatal(err)
	}
	scheduler := &fakeScheduler{}
	service := &Service{
		Tasks: repository, Runs: repository, TaskRuns: repository, Scheduler: scheduler,
		Now: func() time.Time { return createdAt.Add(time.Minute) },
	}
	if err := service.RecoverInterruptedTasks(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.RecoverInterruptedTasks(context.Background()); err != nil {
		t.Fatal(err)
	}
	recovered, err := repository.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != domainsubagent.TaskStatusQueued || recovered.ErrorMessage != "" {
		t.Fatalf("expected recovered task to be queued, got %#v", recovered)
	}
	if len(recovered.Mailbox) != 1 || recovered.Mailbox[0].Status != domainsubagent.MessageStatusPending || recovered.Mailbox[0].ClaimedAt != nil {
		t.Fatalf("expected interrupted mailbox delivery to become pending: %#v", recovered.Mailbox)
	}
	runs, err := repository.ListRuns(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != domainsubagent.RunStatusQueued || runs[0].Instruction != interruptedTaskContinuation {
		t.Fatalf("expected recovered run to be queued, got %#v", runs)
	}
	if scheduler.task == nil || scheduler.submissions != 1 {
		t.Fatalf("expected interrupted task to be submitted once, submissions=%d", scheduler.submissions)
	}
}

func TestRecoverInterruptedTasksRequeuesWaitingStates(t *testing.T) {
	for _, status := range []domainsubagent.TaskStatus{
		domainsubagent.TaskStatusWaitingSubagents,
		domainsubagent.TaskStatusWaitingPermission,
	} {
		t.Run(string(status), func(t *testing.T) {
			repository := memory.NewRepository()
			createdAt := time.Date(2026, 7, 25, 8, 0, 0, 0, time.UTC)
			task := domainsubagent.Task{
				ID: "task-" + string(status), ParentSessionID: "parent-1", Instruction: "original instruction",
				Definition: domainsubagent.DefinitionSnapshot{ModelID: "model-1"}, Status: status,
				CurrentRunID: "run-1", CreatedAt: createdAt, UpdatedAt: createdAt,
			}
			run := domainsubagent.Run{
				ID: "run-1", TaskID: task.ID, Sequence: 1, Instruction: task.Instruction,
				Status: domainsubagent.RunStatusRunning, CreatedAt: createdAt,
			}
			if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
				t.Fatal(err)
			}
			scheduler := &fakeScheduler{}
			service := &Service{Tasks: repository, Runs: repository, TaskRuns: repository, Scheduler: scheduler}
			if err := service.RecoverInterruptedTasks(context.Background()); err != nil {
				t.Fatal(err)
			}
			recovered, err := repository.GetTask(context.Background(), task.ID)
			if err != nil {
				t.Fatal(err)
			}
			runs, err := repository.ListRuns(context.Background(), task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if recovered.Status != domainsubagent.TaskStatusQueued || len(runs) != 1 || runs[0].Instruction != interruptedTaskContinuation || scheduler.submissions != 1 {
				t.Fatalf("waiting task was not recovered: task=%#v runs=%#v submissions=%d", recovered, runs, scheduler.submissions)
			}
		})
	}
}

func TestRecoverInterruptedQueuedTaskKeepsOriginalInstruction(t *testing.T) {
	repository := memory.NewRepository()
	createdAt := time.Date(2026, 7, 25, 8, 0, 0, 0, time.UTC)
	task := domainsubagent.Task{
		ID: "task-queued", ParentSessionID: "parent-1", Instruction: "original instruction",
		Definition: domainsubagent.DefinitionSnapshot{ModelID: "model-1"}, Status: domainsubagent.TaskStatusQueued,
		CurrentRunID: "run-1", CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	run := domainsubagent.Run{
		ID: "run-1", TaskID: task.ID, Sequence: 1, Instruction: task.Instruction,
		Status: domainsubagent.RunStatusQueued, CreatedAt: createdAt,
	}
	if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
		t.Fatal(err)
	}
	service := &Service{Tasks: repository, Runs: repository, TaskRuns: repository, Scheduler: &fakeScheduler{}}
	if err := service.RecoverInterruptedTasks(context.Background()); err != nil {
		t.Fatal(err)
	}
	runs, err := repository.ListRuns(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Instruction != task.Instruction {
		t.Fatalf("queued task instruction changed during recovery: %#v", runs)
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

type fakeScheduler struct {
	task        subagentport.ScheduledTask
	submissions int
}

func (scheduler *fakeScheduler) Submit(_ string, task subagentport.ScheduledTask) error {
	scheduler.task = task
	scheduler.submissions++
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
	creates int
}

func (sessions *fakeChildSessions) Create(_ context.Context, request subagentport.ChildSessionRequest) (*session.Session, error) {
	sessions.request = request
	sessions.creates++
	return session.NewFromState(session.InitialState{ID: request.SessionID, Model: request.FallbackModelID}), nil
}

type fakeRunner struct{}

func (fakeRunner) Run(context.Context, subagentport.AgentRunRequest) (subagentport.AgentRunResult, error) {
	return subagentport.AgentRunResult{Content: "done"}, nil
}

type recordingRunner struct{ instructions []string }

func (runner *recordingRunner) Run(_ context.Context, request subagentport.AgentRunRequest) (subagentport.AgentRunResult, error) {
	runner.instructions = append(runner.instructions, request.Instruction)
	if len(runner.instructions) == 1 {
		return subagentport.AgentRunResult{Content: "initial result"}, nil
	}
	return subagentport.AgentRunResult{Content: "follow-up result"}, nil
}

type failingFollowUpRunner struct{ calls int }

func (runner *failingFollowUpRunner) Run(_ context.Context, _ subagentport.AgentRunRequest) (subagentport.AgentRunResult, error) {
	runner.calls++
	if runner.calls == 1 {
		return subagentport.AgentRunResult{Content: "initial result"}, nil
	}
	return subagentport.AgentRunResult{}, errors.New("follow-up model unavailable")
}

type panicFollowUpRunner struct{ calls int }

func (runner *panicFollowUpRunner) Run(_ context.Context, _ subagentport.AgentRunRequest) (subagentport.AgentRunResult, error) {
	runner.calls++
	if runner.calls == 1 {
		return subagentport.AgentRunResult{Content: "initial result"}, nil
	}
	panic("follow-up panic")
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
