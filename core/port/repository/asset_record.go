package repository

import "time"

const (
	AssetSourceUpload        = "asset_upload"
	AssetSourceAttachImage   = "attach_image"
	AssetSourceShareFile     = "share_file"
	AssetSourceGenerateImage = "generate_image"
)

// IsSessionAttachment excludes metadata-only tool results from authorization.
func (a AssetRecord) IsSessionAttachment() bool {
	if a.Deleted || a.DeletedAt != nil {
		return false
	}
	switch a.ToolName {
	case AssetSourceUpload, AssetSourceAttachImage, AssetSourceShareFile, AssetSourceGenerateImage:
		return true
	default:
		return false
	}
}

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
