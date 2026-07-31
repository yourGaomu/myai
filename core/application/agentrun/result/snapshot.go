package result

import domainagentrun "myai/core/domain/agentrun"

type Snapshot struct {
	Run    domainagentrun.Run
	Events []domainagentrun.Event
}
