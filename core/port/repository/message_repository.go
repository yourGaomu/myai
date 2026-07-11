package repository

import "context"

type MessageRepository interface {
	SaveMessage(ctx context.Context, message MessageRecord) error
	ClearMessages(ctx context.Context, sessionID string) error
	ListMessages(ctx context.Context, sessionID string) ([]MessageRecord, error)
	GetMessageHistoryMeta(ctx context.Context, sessionID string) (MessageHistoryMeta, error)
	ListMessagesAfter(ctx context.Context, sessionID string, afterMessageID string, limit int) ([]MessageRecord, bool, error)
}
