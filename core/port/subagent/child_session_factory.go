package subagent

import (
	"context"

	"myai/core/session"
)

type ChildSessionFactory interface {
	Create(ctx context.Context, request ChildSessionRequest) (*session.Session, error)
}
