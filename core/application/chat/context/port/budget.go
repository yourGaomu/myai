package port

import (
	"myai/core/contextmgr"
	"myai/core/session"
)

type RequestProvider interface {
	SnapshotForRequest(current *session.Session, options contextmgr.RequestOptions) contextmgr.Snapshot
}

type BudgetedProvider interface {
	SnapshotWithBudget(current *session.Session, reservedTokens int) contextmgr.Snapshot
}
