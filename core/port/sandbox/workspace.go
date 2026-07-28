package sandbox

import (
	"context"

	domainsandbox "myai/core/domain/sandbox"
)

type Workspace interface {
	ID() string
	Run(ctx context.Context, request domainsandbox.CommandRequest) (domainsandbox.CommandResult, error)
	CreateDirectory(ctx context.Context, path string, mode uint32) error
	UploadFiles(ctx context.Context, files []domainsandbox.File) error
	DownloadFile(ctx context.Context, path string, maxBytes int64) ([]byte, error)
	Destroy(ctx context.Context) error
	Close() error
}
