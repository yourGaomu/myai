package api

import (
	"context"

	querycommand "myai/core/application/session/query/command"
	sessionresult "myai/core/application/session/result"
)

type SessionQueryService interface {
	ListSessions(ctx context.Context, includeDeleted bool) ([]sessionresult.SessionListItem, error)
	ListAssets(ctx context.Context, command querycommand.ListAssets) ([]sessionresult.AssetListItem, error)
}

type MessageQueryService interface {
	ListMessages(ctx context.Context, sessionID string) ([]sessionresult.MessageListItem, error)
	HistoryMeta(ctx context.Context, sessionID string) (sessionresult.MessageHistoryMeta, error)
	ListMessagesAfter(ctx context.Context, sessionID string, afterMessageID string, limit int) ([]sessionresult.MessageListItem, bool, error)
}
