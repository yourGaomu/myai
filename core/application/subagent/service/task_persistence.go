package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	domainsubagent "myai/core/domain/subagent"
	subagentport "myai/core/port/subagent"
)

const interruptedTaskMessage = "subagent execution was interrupted by process restart"
const interruptedTaskContinuation = "Continue the interrupted task from the existing child session. Review the current state and finish the assigned work."

func (service *Service) saveTaskAndRun(ctx context.Context, task domainsubagent.Task, run domainsubagent.Run) error {
	if service == nil {
		return errors.New("subagent task service is nil")
	}
	if task.ID == "" || run.ID == "" || run.TaskID != task.ID {
		return errors.New("subagent task and run state is inconsistent")
	}
	repository := service.TaskRuns
	if repository == nil {
		if candidate, ok := service.Tasks.(subagentport.TaskRunRepository); ok {
			repository = candidate
		} else if candidate, ok := service.Runs.(subagentport.TaskRunRepository); ok {
			repository = candidate
		}
	}
	if repository == nil {
		return errors.New("atomic subagent task/run repository is not configured")
	}
	return repository.SaveTaskAndRun(ctx, domainsubagent.CloneTask(task), domainsubagent.CloneRun(run))
}

func (service *Service) currentRun(ctx context.Context, task domainsubagent.Task) (domainsubagent.Run, error) {
	if service == nil || service.Runs == nil {
		return domainsubagent.Run{}, errors.New("subagent run repository is nil")
	}
	runs, err := service.Runs.ListRuns(ctx, task.ID)
	if err != nil {
		return domainsubagent.Run{}, err
	}
	for _, run := range runs {
		if run.ID == task.CurrentRunID {
			return domainsubagent.CloneRun(run), nil
		}
	}
	return domainsubagent.Run{}, fmt.Errorf("subagent current run not found: %s", task.CurrentRunID)
}

func (service *Service) finishPanic(taskID string, runID string, panicErr error) error {
	if service == nil || service.Tasks == nil || service.Runs == nil {
		return errors.New("subagent task service is not configured")
	}
	task, err := service.Tasks.GetTask(context.Background(), strings.TrimSpace(taskID))
	if err != nil {
		return err
	}
	if task.Terminal() {
		return nil
	}
	run, err := service.currentRun(context.Background(), task)
	if err != nil {
		return err
	}
	if runID != "" && run.ID != runID {
		return fmt.Errorf("subagent panic run mismatch: expected %s, got %s", runID, run.ID)
	}
	return service.finishError(task, run, context.Background(), panicErr)
}

// RecoverInterruptedTasks reconciles tasks that could not reach a terminal
// state because the previous process exited before its scheduler drained.
func (service *Service) RecoverInterruptedTasks(ctx context.Context) error {
	if service == nil || service.Tasks == nil || service.Runs == nil {
		return errors.New("subagent task service is not configured")
	}
	repository, ok := service.Tasks.(subagentport.InterruptedTaskRepository)
	if !ok {
		return errors.New("subagent interrupted-task repository is not configured")
	}
	tasks, err := repository.ListNonTerminalTasks(ctx)
	if err != nil {
		return err
	}
	var recoveryErrors []error
	for _, task := range tasks {
		if service.taskIsActive(task.ID) {
			continue
		}
		run, runErr := service.recoveryRun(ctx, &task)
		if runErr != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("load interrupted subagent task %s run: %w", task.ID, runErr))
			continue
		}
		if service.canRequeueAfterRestart(task) {
			if err := service.requeueInterruptedTask(ctx, task, run); err == nil {
				continue
			} else {
				recoveryErrors = append(recoveryErrors, fmt.Errorf("requeue interrupted subagent task %s: %w", task.ID, err))
			}
		}
		now := service.now()
		if err := task.MarkFailed(interruptedTaskMessage, now); err != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("fail interrupted subagent task %s: %w", task.ID, err))
			continue
		}
		run.Status = domainsubagent.RunStatusFailed
		run.ErrorMessage = interruptedTaskMessage
		run.CompletedAt = timePtr(now)
		if err := service.saveTaskAndRun(ctx, task, run); err != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("save interrupted subagent task %s: %w", task.ID, err))
			continue
		}
		service.discardFailedWorkspace(task)
		service.publish(ctx, task)
	}
	return errors.Join(recoveryErrors...)
}

func (service *Service) canRequeueAfterRestart(task domainsubagent.Task) bool {
	if service == nil || service.Scheduler == nil {
		return false
	}
	switch task.Status {
	case domainsubagent.TaskStatusQueued, domainsubagent.TaskStatusRunning,
		domainsubagent.TaskStatusWaitingSubagents, domainsubagent.TaskStatusWaitingPermission:
		return true
	default:
		return false
	}
}

// requeueInterruptedTask preserves queued/running work across a process
// restart. The previous scheduler context is gone, so the task is reset to a
// clean queued transition and admitted to the new scheduler. If admission
// fails, the caller falls back to the existing terminal failure path.
func (service *Service) requeueInterruptedTask(ctx context.Context, task domainsubagent.Task, run domainsubagent.Run) error {
	if !service.registerActiveRun(task.ID, run.ID) {
		return nil
	}
	registered := true
	defer func() {
		if registered {
			service.completeActiveRun(task.ID, run.ID)
		}
	}()
	now := service.now()
	wasInterrupted := task.Status != domainsubagent.TaskStatusQueued ||
		run.Status != domainsubagent.RunStatusQueued
	task.RecoverMailboxDeliveries(now)
	task.Status = domainsubagent.TaskStatusQueued
	task.ErrorMessage = ""
	task.StartedAt = nil
	task.CompletedAt = nil
	task.UpdatedAt = now
	run.Status = domainsubagent.RunStatusQueued
	if wasInterrupted {
		run.Instruction = interruptedTaskContinuation
	}
	run.ErrorMessage = ""
	run.StartedAt = nil
	run.CompletedAt = nil
	if err := service.saveTaskAndRun(ctx, task, run); err != nil {
		return err
	}
	service.publish(ctx, task)
	if err := service.Scheduler.Submit(run.ID, func(runContext context.Context) {
		service.execute(runContext, task.ID, run.ID, task.Definition.ModelID)
	}); err != nil {
		return err
	}
	registered = false
	return nil
}

func (service *Service) recoveryRun(ctx context.Context, task *domainsubagent.Task) (domainsubagent.Run, error) {
	if task == nil {
		return domainsubagent.Run{}, errors.New("subagent task is nil")
	}
	runs, err := service.Runs.ListRuns(ctx, task.ID)
	if err != nil {
		return domainsubagent.Run{}, err
	}
	for _, run := range runs {
		if run.ID == task.CurrentRunID {
			return domainsubagent.CloneRun(run), nil
		}
	}
	if task.CurrentRunID == "" {
		if service.IDs == nil {
			return domainsubagent.Run{}, errors.New("subagent interrupted task has no current run id")
		}
		task.CurrentRunID = service.IDs.NewID()
	}
	return domainsubagent.Run{
		ID:          task.CurrentRunID,
		TaskID:      task.ID,
		Sequence:    1,
		Instruction: task.Instruction,
		Status:      domainsubagent.RunStatusQueued,
		CreatedAt:   task.CreatedAt,
	}, nil
}
