package files

import (
	"io"
	"time"
)

const MaxFileBytes int64 = 50 * 1024 * 1024

type UploadRequest struct {
	Reader         io.Reader
	FileName       string
	ContentType    string
	Size           int64
	IdempotencyKey string
}

type ShareRequest struct {
	UploadRequest
	Title      string
	TTLSeconds int64
	MaxVisits  int64
}

type Asset struct {
	AssetID       string
	BusinessID    string
	FileName      string
	ContentType   string
	Size          int64
	Checksum      string
	ShortURL      string
	Code          string
	ExpiresAt     *time.Time
	MaxVisits     int64
	CreatedAt     time.Time
	localPath     string
	workspaceRoot string
}

func (a Asset) WithLocalImage(path, workspace string) Asset {
	a.localPath, a.workspaceRoot = path, workspace
	return a
}
func (a Asset) LocalPath() string     { return a.localPath }
func (a Asset) WorkspaceRoot() string { return a.workspaceRoot }

type DownloadRequest struct {
	AssetID  string
	MaxBytes int64
}

type Download struct {
	Asset     Asset
	Data      []byte
	Truncated bool
}
