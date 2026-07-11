package cache

import (
	"context"
	"time"
)

type CurrentSessionCache interface {
	SetCurrentSession(ctx context.Context, userID string, sessionID string, ttl time.Duration) error
	GetCurrentSession(ctx context.Context, userID string) (string, error)
	DeleteCurrentSession(ctx context.Context, userID string) error
}
