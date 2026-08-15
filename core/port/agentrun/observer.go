package agentrun

import (
	"context"

	domainagentrun "myai/core/domain/agentrun"
)

type CompletionObserver interface {
	AgentRunCompleted(ctx context.Context, run domainagentrun.Run)
}
