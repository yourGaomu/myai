package command

import domainagentrun "myai/core/domain/agentrun"

type Start struct {
	RunID      string
	RequestID  string
	SessionID  string
	Kind       domainagentrun.Kind
	Title      string
	Reason     string
	TotalSteps int
}

type Append struct {
	RunID        string
	Type         domainagentrun.EventType
	Title        string
	Content      string
	ToolName     string
	Arguments    string
	Status       string
	ErrorCode    string
	ErrorMessage string
	Truncated    bool
	CurrentStep  int
	TotalSteps   int
}

type ReplaceEventContent struct {
	RunID     string
	EventID   string
	Content   string
	Truncated bool
}

type Finish struct {
	RunID        string
	Status       domainagentrun.Status
	ErrorMessage string
	CurrentStep  int
	TotalSteps   int
}
