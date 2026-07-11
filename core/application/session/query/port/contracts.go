package port

import (
	"context"

	repository "myai/core/port/repository"
	"myai/core/session"
)

type SessionListRepository interface {
	ListSessionsWithDeleted(ctx context.Context, includeDeleted bool) ([]repository.SessionRecord, error)
}

type SessionAssetRepository interface {
	ListAssets(ctx context.Context, sessionID string, limit int) ([]repository.AssetRecord, error)
}

type MessageQueryStore interface {
	GetSession(ctx context.Context, sessionID string) (repository.SessionRecord, error)
	ListMessages(ctx context.Context, sessionID string) ([]repository.MessageRecord, error)
	GetMessageHistoryMeta(ctx context.Context, sessionID string) (repository.MessageHistoryMeta, error)
	ListMessagesAfter(ctx context.Context, sessionID string, afterMessageID string, limit int) ([]repository.MessageRecord, bool, error)
}

type MemorySessionSource interface {
	GetSession(sessionID string) (*session.Session, error)
}

type MemoryMessageRecordMapper interface {
	MemoryMessages(current *session.Session) []repository.MessageRecord
}
