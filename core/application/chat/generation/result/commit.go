package result

import (
	domainmessage "myai/core/domain/message"
	agentplan "myai/core/plan"
	"myai/core/session"
)

type Commit struct {
	Plan    *agentplan.Plan
	Session *session.Session
	// AssistantMessage is the final message returned by the transcript append.
	AssistantMessage *domainmessage.Message
}
