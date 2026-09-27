package command

import (
	"time"

	domainsubagent "myai/core/domain/subagent"
	modelport "myai/core/port/model"
)

type BootstrapDefinitions struct{}

type CreateDefinition struct {
	Definition domainsubagent.Definition
}

type UpdateDefinition struct {
	Definition domainsubagent.Definition
}

type DeleteDefinition struct {
	DefinitionID string
}

type StartTask struct {
	ParentSessionID  string
	ParentTaskID     string
	ParentRunID      string
	PlanID           string
	StepID           string
	CreatedRequestID string
	DefinitionID     string
	Instruction      string
	Title            string
	TaskName         string
	AgentPath        string
	AgentNickname    string
	ModelID          string
	FallbackModelID  string
	WorkspaceRoot    string
}

type CheckTask struct {
	TaskID          string
	ParentSessionID string
	RequestID       string
}

type SendMessage struct {
	TaskID          string
	ParentSessionID string
	MessageID       string
	Kind            domainsubagent.AgentMessageKind
	Trigger         domainsubagent.AgentMessageTrigger
	Content         string
}

type FollowupTask struct {
	TaskID          string
	ParentSessionID string
	RequestID       string
	Content         string
	FallbackModelID string
}

type ListTasks struct {
	ParentSessionID string
	Limit           int
}

type CancelTask struct {
	TaskID          string
	ParentSessionID string
	Reason          string
}

type ApplyTaskChanges struct {
	TaskID          string
	ParentSessionID string
	RequestID       string
}

type DiscardTaskChanges struct {
	TaskID          string
	ParentSessionID string
}

type ResumeTask struct {
	TaskID          string
	ParentSessionID string
	Stream          modelport.ChatStreamHandler
}

// WaitTask waits for the first target child task to reach a terminal state.
// Targets may contain more than one task ID; the wait completes when any
// target reaches a terminal state. TaskID is kept as a single-target shortcut
// for callers that have not migrated to Targets yet.
type WaitTask struct {
	TaskID          string
	Targets         []string
	ParentSessionID string
	ParentTaskID    string
	Timeout         time.Duration
}
