package result

import (
	"time"

	agentplan "myai/core/plan"
)

type SessionListItem struct {
	ID             string
	Title          string
	Model          string
	AgentMode      string
	PermissionMode string
	ContextWindowK int
	Usage          *TokenUsage
	LastUsage      *TokenUsage
	CurrentPlan    *agentplan.Plan
	Deleted        bool
	DeletedAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
