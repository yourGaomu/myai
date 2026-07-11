package authorization

import (
	"context"
	"time"

	domainauthorization "myai/core/domain/authorization"
)

type Store interface {
	Save(ctx context.Context, authorization domainauthorization.ClientAuthorization) error
	Get(ctx context.Context, id string) (domainauthorization.ClientAuthorization, error)
	Touch(ctx context.Context, id string, lastSeenAt time.Time) error
	Revoke(ctx context.Context, id string, revokedAt time.Time) error
	ListActive(ctx context.Context, userID string, deviceID string) ([]domainauthorization.ClientAuthorization, error)
}
