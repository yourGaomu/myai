package workspace

import (
	"context"

	domainsandbox "myai/core/domain/sandbox"
)

type CommandRunner interface {
	Run(ctx context.Context, sandboxID string, localRoot string, request domainsandbox.CommandRequest) (domainsandbox.CommandResult, error)
}
