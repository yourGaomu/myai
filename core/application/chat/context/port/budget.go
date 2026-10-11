package port

import (
	"myai/core/contextmgr"
	"myai/core/session"
)

type BudgetedProvider interface {
	SnapshotWithBudget(current *session.Session, reservedTokens int) contextmgr.Snapshot
}
