package command

import domainsubagent "myai/core/domain/subagent"

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
	CreatedRequestID string
	DefinitionID     string
	Instruction      string
	Title            string
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
