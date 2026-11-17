package port

import (
	"context"

	compactionresult "myai/core/application/chat/compaction/result"
	modelport "myai/core/port/model"
	"myai/core/session"
)

type Persistence interface {
	PersistAssistant(current *session.Session, result modelport.ChatResult)
}

// DurablePersistence accepts an assistant write before generation reports success.
// Legacy Persistence implementations remain supported for non-durable callers.
type DurablePersistence interface {
	AcceptAssistant(ctx context.Context, current *session.Session, result modelport.ChatResult) error
}

type AutoCompactor interface {
	CompactIfNeeded(ctx context.Context, current *session.Session, model modelport.ChatModelPort) (compactionresult.CompactInfo, error)
}
