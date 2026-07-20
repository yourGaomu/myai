package api

import (
	"context"

	"myai/core/contextmgr"
	"myai/core/session"
)

type SnapshotService interface {
	Snapshot(current *session.Session) contextmgr.Snapshot
}

type QueryService interface {
	Info(ctx context.Context, current *session.Session) contextmgr.Info
}
