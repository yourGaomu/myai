package opensandbox

import (
	"context"
	"io"

	sdk "github.com/alibaba/OpenSandbox/sdks/sandbox/go"
)

type sandboxClient interface {
	ID() string
	RunCommandWithOpts(ctx context.Context, request sdk.RunCommandRequest, handlers *sdk.ExecutionHandlers) (*sdk.Execution, error)
	CreateDirectory(ctx context.Context, path string, mode int) error
	UploadFiles(ctx context.Context, entries []sdk.UploadFileEntry) error
	DownloadFile(ctx context.Context, remotePath string, rangeHeader string, options ...sdk.DownloadFileOptions) (io.ReadCloser, error)
	Kill(ctx context.Context) error
	Close() error
}

type createSandboxFunc func(context.Context, sdk.ConnectionConfig, sdk.SandboxCreateOptions) (sandboxClient, error)
type connectSandboxFunc func(context.Context, sdk.ConnectionConfig, string) (sandboxClient, error)

func createSDKInstance(ctx context.Context, config sdk.ConnectionConfig, options sdk.SandboxCreateOptions) (sandboxClient, error) {
	return sdk.CreateSandbox(ctx, config, options)
}

func connectSDKInstance(ctx context.Context, config sdk.ConnectionConfig, sandboxID string) (sandboxClient, error) {
	return sdk.ConnectSandbox(ctx, config, sandboxID)
}
