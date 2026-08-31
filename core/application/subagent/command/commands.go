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

// WaitTask waits for a child task's next terminal state change. The wait is
// event-driven when the configured publisher supports TaskEventSource.
type WaitTask struct {
	TaskID          string
	ParentSessionID string
	ParentTaskID    string
	Timeout         time.Duration
}
