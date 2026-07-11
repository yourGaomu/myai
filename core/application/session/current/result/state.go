package result

import (
	"myai/core/llm"
	agentplan "myai/core/plan"
	"myai/core/session"
)

type State struct {
	SessionID      string
	ModelID        string
	AgentMode      session.AgentMode
	PermissionMode session.PermissionMode
	ContextWindowK int
	Usage          llm.TokenUsage
	LastUsage      llm.TokenUsage
	Plan           *agentplan.Plan
}
