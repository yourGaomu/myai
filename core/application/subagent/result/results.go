package result

import (
	domainsubagent "myai/core/domain/subagent"
	modelport "myai/core/port/model"
)

type Definitions struct {
	Items []domainsubagent.Definition
}

type Definition struct {
	Value domainsubagent.Definition
}

type Task struct {
	Value domainsubagent.Task
}

type Tasks struct {
	Items []domainsubagent.Task
}

type Resume struct {
	Task      domainsubagent.Task
	Content   string
	Reasoning string
	Usage     modelport.TokenUsage
}

type Wait struct {
	Task           domainsubagent.Task
	TimedOut       bool
	WokenByMailbox bool
	Sequence       uint64
}
