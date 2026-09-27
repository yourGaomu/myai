package subagent

import (
	"context"

	"myai/core/session"
)

type ChildSessionFactory interface {
	Create(ctx context.Context, request ChildSessionRequest) (*session.Session, error)
}

// ChildSessionLifecycle is optional on factories that keep live child
// sessions in process memory. Unload must persist the current snapshot before
// releasing the in-memory runtime; Create is then responsible for reloading it
// on the next turn.
type ChildSessionLifecycle interface {
	Unload(ctx context.Context, sessionID string) error
}
