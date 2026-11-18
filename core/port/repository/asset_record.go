package repository

import "time"

type AssetRecord struct {
	ID          string
	AssetID     string
	BusinessID  string
	Checksum    string
	MaxVisits   int64
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
