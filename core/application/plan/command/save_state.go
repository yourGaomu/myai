package command

import agentplan "myai/core/plan"

type SaveState struct {
	SessionID string
	Model     string
	Plan      *agentplan.Plan
}
