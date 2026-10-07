package repository

import "context"

// SessionContinuationStore keeps explicit user stops independently of async
// session snapshots, which must never overwrite a newer pause decision.
type SessionContinuationStore interface {
	ContinuationAllowed(ctx context.Context, sessionID string) (bool, error)
	SetContinuationPaused(ctx context.Context, sessionID string, paused bool) error
}
