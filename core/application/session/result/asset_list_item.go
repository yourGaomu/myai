package result

import "time"

type AssetListItem struct {
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
	CreatedAt   time.Time
}
