package repository

import "time"

type AssetRecord struct {
	ID          string
	SessionID   string
	RequestID   string
	ToolCallID  string
	ToolName    string
	LocalPath   string
	FileName    string
	ContentType string
	Size        int64
	ShortURL    string
	ShortCode   string
	ExpiresAt   *time.Time
	Deleted     bool
	DeletedAt   *time.Time
	CreatedAt   time.Time
}
