package subagent

import (
	"context"
	repository "myai/core/port/repository"
)

type SessionRecordReader interface {
	GetSession(ctx context.Context, sessionID string) (repository.SessionRecord, error)
}

// TaskEventHistoryRepository 按持久化游标分页，不依赖实时订阅的内存缓冲区。
type TaskEventHistoryRepository interface {
	QueryTaskEvents(ctx context.Context, filter TaskEventFilter) ([]TaskEvent, error)
	LatestTaskEventSequence(ctx context.Context) (uint64, error)
}
