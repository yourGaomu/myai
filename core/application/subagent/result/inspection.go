package result

import (
	domainsubagent "myai/core/domain/subagent"
	subagentport "myai/core/port/subagent"
)

type Inspection struct {
	Task          domainsubagent.Task
	Runs          []domainsubagent.Run
	SessionReady  bool
	EventSequence uint64
}

type EventPage struct {
	Events          []subagentport.TaskEvent
	NextSequence    uint64
	ThroughSequence uint64
	HasMore         bool
}
