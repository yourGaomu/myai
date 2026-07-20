package result

import (
	"time"

	agentplan "myai/core/plan"
	"myai/core/session"
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
	RAGSettings    session.RAGSettings
	Deleted        bool
	DeletedAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
