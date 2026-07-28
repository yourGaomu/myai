package sandbox

import (
	"context"

	domainsandbox "myai/core/domain/sandbox"
)

type Manager interface {
	Create(ctx context.Context, request domainsandbox.CreateRequest) (Workspace, error)
	Connect(ctx context.Context, sandboxID string) (Workspace, error)
	Delete(ctx context.Context, sandboxID string) error
}
