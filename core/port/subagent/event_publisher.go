package subagent

import (
	"context"

	domainsubagent "myai/core/domain/subagent"
)

type EventPublisher interface {
	TaskUpdated(ctx context.Context, task domainsubagent.Task)
}
