package repository

import (
	"time"

	generation "myai/core/domain/generation"
	agentplan "myai/core/plan"
	"myai/core/session"
)

type SessionRecord struct {
	ID                 string
	Model              string
	AgentMode          string
	PermissionMode     string
	ContextWindowK     int
	Summary            string
	CompactedMessages  int
	CompactedAt        *time.Time
	Title              string
	Usage              *TokenUsageRecord
	LastUsage          *TokenUsageRecord
	CurrentPlan        *agentplan.Plan
	RAGSettings        session.RAGSettings
	GenerationSettings generation.Settings
	StyleInstruction   string
	Deleted            bool
	DeletedAt          *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
