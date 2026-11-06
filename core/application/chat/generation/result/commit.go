package result

import (
	agentplan "myai/core/plan"
	"myai/core/session"
)

type Commit struct {
	Plan    *agentplan.Plan
	Session *session.Session
}
