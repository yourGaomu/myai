package subagent

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	domainworkspace "myai/core/domain/workspace"
)

const MaxInstructionRunes = 20000

type TaskStatus string

const (
	TaskStatusQueued            TaskStatus = "queued"
	TaskStatusRunning           TaskStatus = "running"
	TaskStatusWaitingSubagents  TaskStatus = "waiting_subagents"
	TaskStatusWaitingPermission TaskStatus = "waiting_permission"
	TaskStatusSucceeded         TaskStatus = "succeeded"
	TaskStatusFailed            TaskStatus = "failed"
	TaskStatusCanceled          TaskStatus = "canceled"
)

type RunStatus string

const (
	RunStatusQueued    RunStatus = "queued"
	RunStatusRunning   RunStatus = "running"
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCanceled  RunStatus = "canceled"
)

type Task struct {
	ID                string
	ParentSessionID   string
	ChildSessionID    string
	CreatedRequestID  string
	DefinitionID      string
	DefinitionVersion int64
	Definition        DefinitionSnapshot
	Instruction       string
	Title             string
	Status            TaskStatus
	CurrentRunID      string
	Workspace         domainworkspace.Reference
	ChangeSet         domainworkspace.ChangeSet
	Result            string
	Reasoning         string
	ErrorMessage      string
	Unread            bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
	StartedAt         *time.Time
	CompletedAt       *time.Time
}

type Run struct {
	ID           string
	TaskID       string
	Sequence     int
	Instruction  string
	Status       RunStatus
	Result       string
	ErrorMessage string
	CreatedAt    time.Time
	StartedAt    *time.Time
	CompletedAt  *time.Time
}

func (task Task) ValidateNew() error {
	if strings.TrimSpace(task.ID) == "" {
		return errors.New("subagent task id is required")
	}
	if strings.TrimSpace(task.ParentSessionID) == "" {
		return errors.New("subagent parent session id is required")
	}
	if strings.TrimSpace(task.DefinitionID) == "" {
		return errors.New("subagent definition id is required")
	}
	if strings.TrimSpace(task.Instruction) == "" {
		return errors.New("subagent instruction is required")
	}
	if utf8.RuneCountInString(task.Instruction) > MaxInstructionRunes {
		return fmt.Errorf("subagent instruction must not exceed %d characters", MaxInstructionRunes)
	}
	return nil
}

func (task Task) Terminal() bool {
	switch task.Status {
	case TaskStatusSucceeded, TaskStatusFailed, TaskStatusCanceled:
		return true
	default:
		return false
	}
}

func (task *Task) MarkRunning(now time.Time) error {
	if task == nil {
		return errors.New("subagent task is nil")
	}
	if task.Status != TaskStatusQueued {
		return fmt.Errorf("cannot start subagent task in status %s", task.Status)
	}
	task.Status = TaskStatusRunning
	task.StartedAt = timePointer(now)
	task.UpdatedAt = now
	return nil
}

func (task *Task) MarkSucceeded(result string, reasoning string, now time.Time) error {
	if task == nil || task.Terminal() {
		return errors.New("subagent task is already terminal")
	}
	task.Status = TaskStatusSucceeded
	task.Result = strings.TrimSpace(result)
	task.Reasoning = strings.TrimSpace(reasoning)
	task.ErrorMessage = ""
	task.Unread = true
	task.CompletedAt = timePointer(now)
	task.UpdatedAt = now
	return nil
}

func (task *Task) MarkFailed(message string, now time.Time) error {
	if task == nil || task.Terminal() {
		return errors.New("subagent task is already terminal")
	}
	task.Status = TaskStatusFailed
	task.ErrorMessage = strings.TrimSpace(message)
	task.Unread = true
	task.CompletedAt = timePointer(now)
	task.UpdatedAt = now
	return nil
}

func (task *Task) MarkCanceled(message string, now time.Time) error {
	if task == nil || task.Terminal() {
		return errors.New("subagent task is already terminal")
	}
	task.Status = TaskStatusCanceled
	task.ErrorMessage = strings.TrimSpace(message)
	task.Unread = true
	task.CompletedAt = timePointer(now)
	task.UpdatedAt = now
	return nil
}

func CloneTask(task Task) Task {
	task.Definition = CloneDefinitionSnapshot(task.Definition)
	task.ChangeSet = domainworkspace.CloneChangeSet(task.ChangeSet)
	task.StartedAt = cloneTime(task.StartedAt)
	task.CompletedAt = cloneTime(task.CompletedAt)
	return task
}

func CloneRun(run Run) Run {
	run.StartedAt = cloneTime(run.StartedAt)
	run.CompletedAt = cloneTime(run.CompletedAt)
	return run
}

func timePointer(value time.Time) *time.Time {
	copy := value
	return &copy
}
