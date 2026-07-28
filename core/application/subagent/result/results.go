package result

import domainsubagent "myai/core/domain/subagent"

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
