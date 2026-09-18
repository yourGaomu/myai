package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	subagentcommand "myai/core/application/subagent/command"
	subagentresult "myai/core/application/subagent/result"
	domainsubagent "myai/core/domain/subagent"
	domainworkspace "myai/core/domain/workspace"
	subagentport "myai/core/port/subagent"
	workspaceport "myai/core/port/workspace"
)

// Followup preserves the child session and prior runs. Admission waits for
// the previous execution to release the session before creating another run.
func (service *Service) Followup(ctx context.Context, command subagentcommand.FollowupTask) (subagentresult.Task, error) {
	if service == nil || service.Tasks == nil || service.Runs == nil || service.Scheduler == nil || service.IDs == nil {
		return subagentresult.Task{}, errors.New("subagent follow-up service is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	command.Content = strings.TrimSpace(command.Content)
	command.RequestID = strings.TrimSpace(command.RequestID)
	if command.Content == "" {
		return subagentresult.Task{}, errors.New("subagent follow-up message is empty")
	}
	if utf8.RuneCountInString(command.Content) > domainsubagent.MaxInstructionRunes {
		return subagentresult.Task{}, fmt.Errorf("subagent follow-up must not exceed %d characters", domainsubagent.MaxInstructionRunes)
	}
	for {
		service.admissionMu.Lock()
		service.mu.Lock()
		task, run, done, err := service.admitFollowup(ctx, command)
		service.mu.Unlock()
		service.admissionMu.Unlock()
		if err != nil {
			return subagentresult.Task{}, err
		}
		if done != nil {
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return subagentresult.Task{}, ctx.Err()
			}
		}
		if run.ID == "" {
			return subagentresult.Task{Value: domainsubagent.CloneTask(task)}, nil
		}
		return service.scheduleRun(task, run, command.FallbackModelID)
	}
}

// Caller holds admissionMu and mu, including during the durable admission.
func (service *Service) admitFollowup(ctx context.Context, command subagentcommand.FollowupTask) (task domainsubagent.Task, run domainsubagent.Run, done <-chan struct{}, err error) {
	if err = ctx.Err(); err != nil {
		return
	}
	task, err = service.Tasks.GetTask(ctx, strings.TrimSpace(command.TaskID))
	if err != nil {
		return
	}
	if parentID := strings.TrimSpace(command.ParentSessionID); parentID != "" && task.ParentSessionID != parentID {
		err = subagentport.ErrNotFound
		return
	}
	if command.RequestID != "" && command.RequestID == task.CreatedRequestID {
		err = errors.New("a subagent task cannot be followed up from the request that created it")
		return
	}
	runs, listErr := service.Runs.ListRuns(ctx, task.ID)
	if listErr != nil {
		err = listErr
		return
	}
	nextSequence := 1
	contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(command.Content)))
	for _, previous := range runs {
		if command.RequestID != "" && previous.RequestID == command.RequestID {
			previousHash := previous.RequestContentHash
			if previousHash == "" {
				previousHash = fmt.Sprintf("%x", sha256.Sum256([]byte(previous.Instruction)))
			}
			if previousHash != contentHash {
				err = errors.New("subagent follow-up request id was already used with different content")
			}
			return
		}
		if previous.Sequence >= nextSequence {
			nextSequence = previous.Sequence + 1
		}
	}
	if task.Status != domainsubagent.TaskStatusSucceeded && task.Status != domainsubagent.TaskStatusFailed {
		err = fmt.Errorf("subagent task cannot be followed up in status %s", task.Status)
		return
	}
	if _, claimed := service.resumeClaims[task.ID]; claimed {
		err = errors.New("subagent task result is being consumed by parent continuation")
		return
	}
	if active := service.activeRuns[task.ID]; active != nil {
		done = active.done
		return
	}
	if err = service.validateFollowupWorkspace(ctx, task); err != nil {
		return
	}
	now := service.now()
	run = domainsubagent.Run{
		ID: service.IDs.NewID(), TaskID: task.ID, Sequence: nextSequence,
		RequestID: command.RequestID, Instruction: command.Content,
		RequestContentHash: contentHash,
		Status:             domainsubagent.RunStatusQueued, CreatedAt: now,
	}
	if !service.registerActiveRunLocked(task.ID, run.ID) {
		err = errors.New("subagent task already has an active execution")
		return
	}
	task.CurrentRunID = run.ID
	task.Status = domainsubagent.TaskStatusQueued
	task.Result = ""
	task.Reasoning = ""
	task.ErrorMessage = ""
	task.Unread = false
	task.StartedAt = nil
	task.CompletedAt = nil
	task.UpdatedAt = now
	if err = service.saveTaskAndRun(ctx, task, run); err != nil {
		service.completeActiveRunLocked(task.ID, run.ID)
		return
	}
	service.publish(ctx, task)
	return
}

func (service *Service) validateFollowupWorkspace(ctx context.Context, task domainsubagent.Task) error {
	if task.Workspace.Mode == domainworkspace.IsolationModeDirect {
		return nil
	}
	if service.Workspaces == nil {
		return errors.New("isolated workspace manager is not configured")
	}
	collected, err := service.Workspaces.Collect(ctx, workspaceport.CollectRequest{Reference: task.Workspace})
	if err == nil {
		switch collected.ChangeSet.Status {
		case domainworkspace.ChangeSetStatusPending, domainworkspace.ChangeSetStatusConflict, domainworkspace.ChangeSetStatusApplied:
			return nil
		}
	}
	if workspaceSourceRoot(task) == "" {
		if err != nil {
			return fmt.Errorf("subagent follow-up workspace is unavailable: %w", err)
		}
		return errors.New("subagent isolated workspace source is unavailable for follow-up; start a new task")
	}
	return nil
}
