package result

import "time"

type MessageHistoryMeta struct {
	SessionID            string
	MessageCount         int64
	LastMessageID        string
	LastMessageCreatedAt *time.Time
	HistoryVersion       int64
}
