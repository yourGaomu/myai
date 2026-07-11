package port

import (
	"context"

	compactionresult "myai/core/application/chat/compaction/result"
	modelport "myai/core/port/model"
	"myai/core/session"
)

type Persistence interface {
	PersistAssistant(current *session.Session, result modelport.ChatResult)
	PersistCurrentSession(sessionID string)
}

type AutoCompactor interface {
	CompactIfNeeded(ctx context.Context, current *session.Session, model modelport.ChatModelPort, runtimePrompt string) (compactionresult.CompactInfo, error)
}
