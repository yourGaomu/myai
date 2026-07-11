package port

import "context"

type EventPublisher interface {
	SessionChanged(ctx context.Context, sessionID string, reason string)
}
