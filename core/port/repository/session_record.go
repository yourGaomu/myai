package repository

import (
	"time"

	agentplan "myai/core/plan"
)

type SessionRecord struct {
	ID                string
	Model             string
	AgentMode         string
	PermissionMode    string
	ContextWindowK    int
	Summary           string
	CompactedMessages int
	CompactedAt       *time.Time
	Title             string
	Usage             *TokenUsageRecord
	LastUsage         *TokenUsageRecord
	CurrentPlan       *agentplan.Plan
	Deleted           bool
	DeletedAt         *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
