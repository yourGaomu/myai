package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"myai/core/adapter/subagent/memory"
	subagentcommand "myai/core/application/subagent/command"
	domainsubagent "myai/core/domain/subagent"
	domainworkspace "myai/core/domain/workspace"
	subagentport "myai/core/port/subagent"
)

func TestQueuedRunsAcquireLeaseAtWorkerStartAfterAdmissionExpires(t *testing.T) {
	for _, entry := range []string{"start", "followup", "recovery"} {
		t.Run(entry, func(t *testing.T) {
			repository := memory.NewRepository()
			registry := memory.NewRegistry()
			definition := writableDefinition()
			definition.CapabilityMode = domainsubagent.CapabilityModeReadOnly
			definition.IsolationMode = domainworkspace.IsolationModeDirect
			if err := registry.Register(definition); err != nil {
				t.Fatal(err)
			}
			scheduler := &fakeScheduler{}
			runner := &recordingRunner{}
			service := &Service{Tasks: repository, Runs: repository, TaskRuns: repository, Registry: registry,
				Scheduler: scheduler, Runner: runner, Sessions: &fakeChildSessions{}, IDs: &sequenceIDs{},
				ExecutionLease: repository, ExecutionOwnerID: "worker", ExecutionLeaseTTL: 90 * time.Millisecond,
				AgentMessages: repository}
			var task domainsubagent.Task
			if entry == "start" {
				result, err := service.Start(context.Background(), subagentcommand.StartTask{ParentSessionID: "parent", DefinitionID: definition.ID, Instruction: "initial"})
				if err != nil {
					t.Fatal(err)
				}
				task = result.Value
			} else {
				task = domainsubagent.Task{ID: "task", ParentSessionID: "parent", ChildSessionID: "child", CurrentRunID: "first-run",
					Definition: definition.Snapshot(), Status: domainsubagent.TaskStatusSucceeded,
					Workspace: domainworkspace.Reference{Mode: domainworkspace.IsolationModeDirect}, Instruction: "initial"}
				run := domainsubagent.Run{ID: task.CurrentRunID, TaskID: task.ID, Sequence: 1, Status: domainsubagent.RunStatusSucceeded, Instruction: "initial"}
				if entry == "recovery" {
					task.Status, run.Status = domainsubagent.TaskStatusQueued, domainsubagent.RunStatusQueued
				}
				if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
					t.Fatal(err)
				}
				if entry == "followup" {
					result, err := service.Followup(context.Background(), subagentcommand.FollowupTask{TaskID: task.ID, Content: "follow-up", RequestID: "request"})
					if err != nil {
						t.Fatal(err)
					}
					task = result.Value
				} else if err := service.RecoverInterruptedTasks(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			message := subagentcommand.SendMessage{TaskID: task.ID, ParentSessionID: task.ParentSessionID, MessageID: "mailbox", Content: "queued input"}
			for range 2 {
				if _, err := service.SendMessage(context.Background(), message); err != nil {
					t.Fatal(err)
				}
			}
			// No worker has started. Let the short admission lease expire to
			// model a queue wait longer than the production 30-second TTL.
			time.Sleep(2 * service.ExecutionLeaseTTL)
			if err := repository.Renew(context.Background(), task.ID, task.CurrentRunID, service.ExecutionOwnerID, service.leaseTTL()); !errors.Is(err, subagentport.ErrExecutionLeaseNotAcquired) {
				t.Fatalf("test did not expire the admission lease: %v", err)
			}
			scheduler.task(context.Background())
			current, err := repository.GetTask(context.Background(), task.ID)
			if err != nil || current.Status != domainsubagent.TaskStatusSucceeded || service.taskIsActive(task.ID) {
				t.Fatalf("expired queue lease stranded task: %#v, %v", current, err)
			}
			if len(runner.instructions) != 2 || runner.instructions[1] != message.Content || len(current.Mailbox) != 0 {
				t.Fatalf("mailbox was lost or duplicated: instructions=%v mailbox=%#v", runner.instructions, current.Mailbox)
			}
			if _, err := service.SendMessage(context.Background(), message); err != nil {
				t.Fatalf("delivered mailbox retry lost idempotency: %v", err)
			}
			envelope, err := repository.GetAgentMessage(context.Background(), message.MessageID)
			if err != nil || envelope.Status != domainsubagent.AgentMessageDelivered || envelope.DeliveryAttempts != 1 {
				t.Fatalf("mailbox receipt changed: %#v, %v", envelope, err)
			}
			if err := repository.Acquire(context.Background(), task.ID, task.CurrentRunID, "next-owner", time.Minute); err != nil {
				t.Fatalf("completed worker leaked lease: %v", err)
			}
		})
	}
}

func TestQueuedWorkerDoesNotWriteOrReleaseAnotherOwnersLease(t *testing.T) {
	repository := memory.NewRepository()
	task, run := queuedLeaseTestTask("task", 0)
	if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
		t.Fatal(err)
	}
	if err := repository.Acquire(context.Background(), task.ID, run.ID, "new-owner", time.Minute); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	var reported error
	service := &Service{Tasks: repository, Runs: repository, TaskRuns: repository, ExecutionLease: repository,
		ExecutionOwnerID: "old-owner", Runner: runner, Sessions: &fakeChildSessions{}, OnError: func(err error) { reported = err }}
	if !service.registerActiveRun(task.ID, run.ID) {
		t.Fatal("local registration failed")
	}
	service.execute(context.Background(), task.ID, run.ID, "model")
	current, err := repository.GetTask(context.Background(), task.ID)
	if err != nil || current.Status != domainsubagent.TaskStatusQueued || len(runner.instructions) != 0 || service.taskIsActive(task.ID) {
		t.Fatalf("old queued worker modified task: %#v, %v", current, err)
	}
	if !errors.Is(reported, subagentport.ErrExecutionLeaseNotAcquired) {
		t.Fatalf("lease conflict was not reported: %v", reported)
	}
	if err := repository.Renew(context.Background(), task.ID, run.ID, "new-owner", time.Minute); err != nil {
		t.Fatalf("old worker released another owner's lease: %v", err)
	}
}

func TestRecoveredTasksAreAdmittedInRetryableBatches(t *testing.T) {
	repository := memory.NewRepository()
	scheduler := &boundedRecoveryScheduler{capacity: 2, jobs: make(map[string]subagentport.ScheduledTask)}
	runner := &recordingRunner{}
	service := &Service{Tasks: repository, Runs: repository, TaskRuns: repository, ExecutionLease: repository,
		ExecutionOwnerID: "recovery", Scheduler: scheduler, Runner: runner, Sessions: &fakeChildSessions{}}
	for index := range 7 {
		task, run := queuedLeaseTestTask(fmt.Sprintf("task-%d", index), index)
		if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
			t.Fatal(err)
		}
	}
	for batch := range 4 {
		if err := service.RecoverInterruptedTasks(context.Background()); err != nil {
			t.Fatalf("batch %d failed startup: %v", batch, err)
		}
		want := 2
		if batch == 3 {
			want = 1
		}
		if len(scheduler.jobs) != want {
			t.Fatalf("batch %d admitted %d tasks, want %d", batch, len(scheduler.jobs), want)
		}
		before := len(scheduler.jobs)
		if err := service.RecoverInterruptedTasks(context.Background()); err != nil || len(scheduler.jobs) != before {
			t.Fatalf("recovery duplicated active jobs: err=%v jobs=%d", err, len(scheduler.jobs))
		}
		for _, runID := range append([]string(nil), scheduler.order...) {
			job := scheduler.jobs[runID]
			delete(scheduler.jobs, runID)
			job(context.Background())
		}
		scheduler.order = nil
	}
	if len(runner.instructions) != 7 {
		t.Fatalf("recovered work ran %d times, want 7", len(runner.instructions))
	}
	if err := service.RecoverInterruptedTasks(context.Background()); err != nil || len(scheduler.jobs) != 0 {
		t.Fatalf("finished tasks were requeued: %v", err)
	}
}

func TestRecoveryDefersOnlySchedulerBackpressure(t *testing.T) {
	for _, admissionErr := range []error{subagentport.ErrSchedulerQueueFull, subagentport.ErrSchedulerClosed, errors.New("scheduler failure")} {
		t.Run(admissionErr.Error(), func(t *testing.T) {
			repository := memory.NewRepository()
			for index := range 3 {
				task, run := queuedLeaseTestTask(fmt.Sprintf("task-%d", index), index)
				if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
					t.Fatal(err)
				}
			}
			scheduler := &boundedRecoveryScheduler{err: admissionErr}
			service := &Service{Tasks: repository, Runs: repository, TaskRuns: repository, Scheduler: scheduler,
				ExecutionLease: repository, ExecutionOwnerID: "recovery"}
			err := service.RecoverInterruptedTasks(context.Background())
			retryable := admissionErr == subagentport.ErrSchedulerQueueFull || admissionErr == subagentport.ErrSchedulerClosed
			if (err == nil) != retryable {
				t.Fatalf("recovery error misclassified: %v", err)
			}
			if retryable && scheduler.submissions != 1 {
				t.Fatalf("recovery continued past a full/closed queue: %d", scheduler.submissions)
			}
			for index := range 3 {
				taskID := fmt.Sprintf("task-%d", index)
				current, err := repository.GetTask(context.Background(), taskID)
				if err != nil || current.Terminal() || service.taskIsActive(taskID) {
					t.Fatalf("backpressure lost retryable state: %#v, %v", current, err)
				}
				if err := repository.Acquire(context.Background(), taskID, current.CurrentRunID, "other", time.Minute); err != nil {
					t.Fatalf("rejected recovery leaked lease: %v", err)
				}
			}
		})
	}
}

func TestRecoveryReleasesInterruptedMailboxClaimsWithLease(t *testing.T) {
	for _, envelopes := range []bool{false, true} {
		t.Run(fmt.Sprintf("durable_envelopes=%t", envelopes), func(t *testing.T) {
			repository := memory.NewRepository()
			task, run := queuedLeaseTestTask("task", 0)
			task.Status, run.Status = domainsubagent.TaskStatusRunning, domainsubagent.RunStatusRunning
			if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
				t.Fatal(err)
			}
			scheduler := &fakeScheduler{}
			runner := &recordingRunner{}
			notifier := &recordingParentNotifier{}
			service := &Service{Tasks: repository, Runs: repository, TaskRuns: repository, ExecutionLease: repository,
				ExecutionOwnerID: "recovery", Scheduler: scheduler, Runner: runner, Sessions: &fakeChildSessions{},
				IDs: &sequenceIDs{}, ParentNotifier: notifier}
			if envelopes {
				service.AgentMessages = repository
			}
			message := subagentcommand.SendMessage{TaskID: task.ID, MessageID: "mailbox", Content: "interrupted input"}
			if _, err := service.SendMessage(context.Background(), message); err != nil {
				t.Fatal(err)
			}
			claimed, ok, err := service.claimMailboxMessage(task.ID)
			if err != nil || !ok || claimed.DeliveryAttempts != 1 {
				t.Fatalf("failed to set up interrupted delivery: %#v, %v", claimed, err)
			}
			if err := service.RecoverInterruptedTasks(context.Background()); err != nil {
				t.Fatal(err)
			}
			current, err := repository.GetTask(context.Background(), task.ID)
			if err != nil || len(current.Mailbox) != 1 || current.Mailbox[0].Status != domainsubagent.MessageStatusPending || current.Mailbox[0].ClaimedAt != nil {
				t.Fatalf("recovery retained an interrupted claim: %#v, %v", current.Mailbox, err)
			}
			if err := service.RecoverInterruptedTasks(context.Background()); err != nil || scheduler.submissions != 1 {
				t.Fatalf("repeat scan duplicated recovery: submissions=%d, %v", scheduler.submissions, err)
			}
			scheduler.task(context.Background())
			current, err = repository.GetTask(context.Background(), task.ID)
			if err != nil || current.Status != domainsubagent.TaskStatusSucceeded || len(current.Mailbox) != 0 || len(runner.instructions) != 2 {
				t.Fatalf("recovered delivery failed: task=%#v instructions=%v, %v", current, runner.instructions, err)
			}
			if err := service.ensureParentCompletionQueued(current); err != nil || len(notifier.tasks) != 1 {
				t.Fatalf("completion receipt was duplicated: %#v, %v", notifier.tasks, err)
			}
			if envelopes {
				envelope, err := repository.GetAgentMessage(context.Background(), message.MessageID)
				if err != nil || envelope.Status != domainsubagent.AgentMessageDelivered || envelope.DeliveryAttempts != 2 {
					t.Fatalf("mailbox receipt was not preserved through recovery: %#v, %v", envelope, err)
				}
				if _, err := service.SendMessage(context.Background(), message); err != nil || len(runner.instructions) != 2 {
					t.Fatalf("retry changed delivered receipt: %v", err)
				}
			}
		})
	}
}

func TestRecoveryDoesNotSuppressRepositoryBackpressureShapedErrors(t *testing.T) {
	repository := memory.NewRepository()
	task, run := queuedLeaseTestTask("task", 0)
	if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
		t.Fatal(err)
	}
	failure := subagentport.ErrSchedulerQueueFull
	guarded := &recoveryPersistenceFailure{Repository: repository, err: failure}
	service := &Service{Tasks: guarded, Runs: repository, TaskRuns: guarded, Scheduler: &fakeScheduler{},
		ExecutionLease: repository, ExecutionOwnerID: "recovery"}
	if err := service.RecoverInterruptedTasks(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("repository failure was hidden as scheduler backpressure: %v", err)
	}
}

func TestTaskMutationReleasesLeaseOnReloadEarlyReturns(t *testing.T) {
	for _, operation := range []string{"apply", "discard"} {
		for _, cause := range []string{"reload error", "state conflict"} {
			t.Run(operation+"/"+cause, func(t *testing.T) {
				repository := memory.NewRepository()
				task := domainsubagent.Task{ID: "task", ParentSessionID: "parent", CurrentRunID: "run", Status: domainsubagent.TaskStatusSucceeded,
					Workspace: domainworkspace.Reference{ID: "workspace", Mode: domainworkspace.IsolationModeSnapshot, Root: "snapshot"}, UpdatedAt: time.Now().UTC()}
				if err := repository.SaveTask(context.Background(), task); err != nil {
					t.Fatal(err)
				}
				failure := errors.New("reload failed")
				wrapped := &reloadMutationRepository{Repository: repository, conflict: cause == "state conflict", err: failure}
				service := &Service{Tasks: wrapped, ExecutionLease: repository, ExecutionOwnerID: "api", Workspaces: &fakeWorkspaceManager{}}
				var err error
				if operation == "apply" {
					_, err = service.ApplyChanges(context.Background(), subagentcommand.ApplyTaskChanges{TaskID: task.ID})
				} else {
					_, err = service.DiscardChanges(context.Background(), subagentcommand.DiscardTaskChanges{TaskID: task.ID})
				}
				wantErr := failure
				if wrapped.conflict {
					wantErr = subagentport.ErrTaskStateConflict
				}
				if !errors.Is(err, wantErr) {
					t.Fatalf("expected early return %v, got %v", wantErr, err)
				}
				if err := repository.Acquire(context.Background(), task.ID, "next-operation", "next-owner", time.Minute); err != nil {
					t.Fatalf("%s leaked its operation lease: %v", operation, err)
				}
				current, err := repository.GetTask(context.Background(), task.ID)
				if err != nil || !reflect.DeepEqual(current.Workspace, task.Workspace) {
					t.Fatalf("early return modified workspace: %#v, %v", current, err)
				}
			})
		}
	}
}

func queuedLeaseTestTask(id string, index int) (domainsubagent.Task, domainsubagent.Run) {
	now := time.Date(2026, 11, 6, 12, 0, index, 0, time.UTC)
	task := domainsubagent.Task{ID: id, ParentSessionID: "parent", ChildSessionID: "session-" + id, CurrentRunID: "run-" + id,
		Status: domainsubagent.TaskStatusQueued, Instruction: "work", Definition: domainsubagent.DefinitionSnapshot{TimeoutSeconds: 10},
		Workspace: domainworkspace.Reference{Mode: domainworkspace.IsolationModeDirect}, CreatedAt: now, UpdatedAt: now}
	return task, domainsubagent.Run{ID: task.CurrentRunID, TaskID: task.ID, Status: domainsubagent.RunStatusQueued, Instruction: task.Instruction, CreatedAt: now}
}

type boundedRecoveryScheduler struct {
	capacity, submissions int
	jobs                  map[string]subagentport.ScheduledTask
	order                 []string
	err                   error
}

func (scheduler *boundedRecoveryScheduler) Submit(id string, run subagentport.ScheduledTask) error {
	scheduler.submissions++
	if scheduler.err != nil {
		return scheduler.err
	}
	if len(scheduler.jobs) >= scheduler.capacity {
		return subagentport.ErrSchedulerQueueFull
	}
	if _, exists := scheduler.jobs[id]; exists {
		return errors.New("duplicate scheduled run")
	}
	scheduler.jobs[id] = run
	scheduler.order = append(scheduler.order, id)
	return nil
}
func (*boundedRecoveryScheduler) Cancel(string) bool  { return false }
func (*boundedRecoveryScheduler) Park(string) error   { return nil }
func (*boundedRecoveryScheduler) Unpark(string) error { return nil }

type recoveryPersistenceFailure struct {
	*memory.Repository
	err error
}

func (repository *recoveryPersistenceFailure) SaveOwnedTaskAndRun(context.Context, domainsubagent.Task, domainsubagent.Run, string, time.Duration) error {
	return repository.err
}

type reloadMutationRepository struct {
	*memory.Repository
	reads    int
	conflict bool
	err      error
}

func (repository *reloadMutationRepository) GetTask(ctx context.Context, id string) (domainsubagent.Task, error) {
	repository.reads++
	task, err := repository.Repository.GetTask(ctx, id)
	if repository.reads == 2 && err == nil {
		if !repository.conflict {
			return domainsubagent.Task{}, repository.err
		}
		task.UpdatedAt = task.UpdatedAt.Add(time.Second)
	}
	return task, err
}
