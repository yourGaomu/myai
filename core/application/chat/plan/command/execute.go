package command

import (
	agentplan "myai/core/plan"
	modelport "myai/core/port/model"
)

type Execute struct {
	SessionID   string
	ParentRunID string
	Stream      modelport.ChatStreamHandler
}

type RecoveryRequest struct {
	Plan    *agentplan.Plan
	Step    agentplan.Step
	Error   string
	Attempt int
}
