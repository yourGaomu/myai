package execution

import (
	"context"

	domainexecution "myai/core/domain/execution"
)

// CommandExecutor executes commands but does not imply an isolation boundary.
// Isolation is an explicit property of each RunResult.
type CommandExecutor interface {
	Run(ctx context.Context, request domainexecution.RunRequest) (domainexecution.RunResult, error)
}
