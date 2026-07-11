package api

import (
	"context"

	compactioncommand "myai/core/application/chat/compaction/command"
	compactionresult "myai/core/application/chat/compaction/result"
	"myai/core/contextmgr"
	modelport "myai/core/port/model"
	"myai/core/session"
)

type Compactor interface {
	CompactSession(ctx context.Context, current *session.Session, model modelport.ChatModelPort) error
	CompactIfNeeded(ctx context.Context, current *session.Session, model modelport.ChatModelPort, runtimePrompt string) (compactionresult.CompactInfo, error)
}

type SessionService interface {
	Compact(ctx context.Context, command compactioncommand.CompactSession) (contextmgr.Info, error)
}
