package repository

import (
	"context"

	agentplan "myai/core/plan"
)

// SessionMessageMetadataStore updates only fields owned by message persistence.
// Sequence orders usage snapshots and prevents a delayed retry from reverting it.
type SessionMessageMetadataStore interface {
	TouchSessionMessages(ctx context.Context, record SessionRecord, sequence int64, includeUsage bool) error
}

// SessionPlanStore compares the persisted plan identity and revision before
// replacing it. A mismatch returns ErrConflict without resurrecting old work.
type SessionPlanStore interface {
	SaveSessionPlan(ctx context.Context, sessionID, expectedID string, expectedRevision int64, next *agentplan.Plan) error
}
