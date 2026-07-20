package port

import (
	"myai/core/contextmgr"
	"myai/core/session"
)

type Provider interface {
	Snapshot(current *session.Session) contextmgr.Snapshot
}
