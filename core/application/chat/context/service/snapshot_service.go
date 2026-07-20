package service

import (
	chatcontextapi "myai/core/application/chat/context/api"
	"myai/core/contextmgr"
	"myai/core/session"
)

type SnapshotService struct{}

var _ chatcontextapi.SnapshotService = SnapshotService{}

func (SnapshotService) Snapshot(current *session.Session) contextmgr.Snapshot {
	if current == nil {
		return contextmgr.Snapshot{}
	}
	return contextmgr.BuildSnapshot(current.Messages, current.Summary, current.CompactedMessages, current.ContextWindowK)
}
