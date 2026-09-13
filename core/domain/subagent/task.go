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
const MaxMailboxMessageRunes = 20000
const MaxMailboxMessages = 64
const MaxMailboxRunes = 100000

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
	ParentTaskID      string
	ParentRunID       string
	PlanID            string
	StepID            string
	ChildSessionID    string
	AgentPath         string
	AgentNickname     string
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
	// Mailbox contains durable follow-up input sent by the parent. A runner
	// claims one message at a time and either acknowledges or releases it.
	Mailbox     []Message
	CreatedAt   time.Time
	UpdatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
}

type Message struct {
	ID               string
	Content          string
	Status           MessageStatus
	DeliveryAttempts int
	LastError        string
	CreatedAt        time.Time
	ClaimedAt        *time.Time
}

type MessageStatus string

const (
	MessageStatusPending    MessageStatus = "pending"
	MessageStatusDelivering MessageStatus = "delivering"
)

func (task *Task) EnqueueMessage(message Message) error {
	if task == nil {
		return errors.New("subagent task is nil")
	}
	if task.Terminal() {
		return fmt.Errorf("cannot send message to terminal subagent task")
	}
	if task.Status != TaskStatusQueued && task.Status != TaskStatusRunning && task.Status != TaskStatusWaitingSubagents {
		return fmt.Errorf("cannot send message to subagent task in status %s", task.Status)
	}
	message.Content = strings.TrimSpace(message.Content)
	if message.Content == "" {
		return errors.New("subagent mailbox message is empty")
	}
	if utf8.RuneCountInString(message.Content) > MaxMailboxMessageRunes {
		return fmt.Errorf("subagent mailbox message must not exceed %d characters", MaxMailboxMessageRunes)
	}
	if len(task.Mailbox) >= MaxMailboxMessages {
		return fmt.Errorf("subagent mailbox must not exceed %d messages", MaxMailboxMessages)
	}
	if task.mailboxRunes()+utf8.RuneCountInString(message.Content) > MaxMailboxRunes {
		return fmt.Errorf("subagent mailbox must not exceed %d total characters", MaxMailboxRunes)
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}
	message.Status = MessageStatusPending
	message.ClaimedAt = nil
	message.LastError = ""
	task.Mailbox = append(task.Mailbox, message)
	task.UpdatedAt = message.CreatedAt
	return nil
}

func (task *Task) ClaimMailboxMessage(now time.Time) (Message, bool) {
	if task == nil {
		return Message{}, false
	}
	for index := range task.Mailbox {
		message := &task.Mailbox[index]
		if message.Status != "" && message.Status != MessageStatusPending {
			continue
		}
		message.Status = MessageStatusDelivering
		message.DeliveryAttempts++
		message.LastError = ""
		message.ClaimedAt = timePointer(now)
		task.UpdatedAt = now
		return cloneMessage(*message), true
	}
	return Message{}, false
}

func (task *Task) AcknowledgeMailboxMessage(messageID string, now time.Time) bool {
	if task == nil {
		return false
	}
	for index, message := range task.Mailbox {
		if message.ID != messageID || message.Status != MessageStatusDelivering {
			continue
		}
		task.Mailbox = append(task.Mailbox[:index], task.Mailbox[index+1:]...)
		task.UpdatedAt = now
		return true
	}
	return false
}

func (task *Task) ReleaseMailboxMessage(messageID string, deliveryErr error, now time.Time) bool {
	if task == nil {
		return false
	}
	for index := range task.Mailbox {
		message := &task.Mailbox[index]
		if message.ID != messageID || message.Status != MessageStatusDelivering {
			continue
		}
		message.Status = MessageStatusPending
		message.ClaimedAt = nil
		message.LastError = ""
		if deliveryErr != nil {
			message.LastError = strings.TrimSpace(deliveryErr.Error())
		}
		task.UpdatedAt = now
		return true
	}
	return false
}

func (task *Task) RecoverMailboxDeliveries(now time.Time) bool {
	if task == nil {
		return false
	}
	changed := false
	for index := range task.Mailbox {
		message := &task.Mailbox[index]
		if message.Status == "" {
			message.Status = MessageStatusPending
			changed = true
		}
		if message.Status == MessageStatusDelivering {
			message.Status = MessageStatusPending
			message.ClaimedAt = nil
			changed = true
		}
	}
	if changed {
		task.UpdatedAt = now
	}
	return changed
}

func (task Task) HasMailboxMessages() bool {
	return len(task.Mailbox) > 0
}

func (task Task) mailboxRunes() int {
	total := 0
	for _, message := range task.Mailbox {
		total += utf8.RuneCountInString(message.Content)
	}
	return total
}

type Run struct {
	ID                 string
	TaskID             string
	Sequence           int
	RequestID          string
	RequestContentHash string
	Instruction        string
	Status             RunStatus
	Result             string
	ErrorMessage       string
	CreatedAt          time.Time
	StartedAt          *time.Time
	CompletedAt        *time.Time
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

// CanFollowup describes persisted eligibility. Admission also checks that
// the workspace still exists and the previous execution has released it.
func (task Task) CanFollowup() bool {
	if task.Status != TaskStatusSucceeded && task.Status != TaskStatusFailed {
		return false
	}
	if task.Workspace.Mode == domainworkspace.IsolationModeDirect {
		return true
	}
	return task.Status == TaskStatusSucceeded &&
		(task.ChangeSet.Status == domainworkspace.ChangeSetStatusPending || task.ChangeSet.Status == domainworkspace.ChangeSetStatusConflict)
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
	if task.HasMailboxMessages() {
		return errors.New("subagent task still has mailbox messages")
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
	task.Unread = false
	task.CompletedAt = timePointer(now)
	task.UpdatedAt = now
	return nil
}

func CloneTask(task Task) Task {
	task.Definition = CloneDefinitionSnapshot(task.Definition)
	task.ChangeSet = domainworkspace.CloneChangeSet(task.ChangeSet)
	task.StartedAt = cloneTime(task.StartedAt)
	task.CompletedAt = cloneTime(task.CompletedAt)
	if len(task.Mailbox) > 0 {
		mailbox := make([]Message, len(task.Mailbox))
		for index, message := range task.Mailbox {
			mailbox[index] = cloneMessage(message)
		}
		task.Mailbox = mailbox
	}
	return task
}

func cloneMessage(message Message) Message {
	message.ClaimedAt = cloneTime(message.ClaimedAt)
	return message
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
