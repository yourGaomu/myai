package subagent

import (
	"context"

	domainsubagent "myai/core/domain/subagent"
)

type EventPublisher interface {
	TaskUpdated(ctx context.Context, task domainsubagent.Task)
}

// TaskEventPublisher is implemented by event buses that can carry runtime
// output in addition to task state transitions. It is optional so older
// integrations that only implement EventPublisher keep working.
type TaskEventPublisher interface {
	PublishTaskEvent(ctx context.Context, event TaskEvent)
}

// TaskEventSource is optional on EventPublisher implementations. Keeping it
// separate preserves compatibility with existing publishers and tests.
type TaskEventSource interface {
	SubscribeTaskEvents(parentSessionID string, afterSequence uint64, buffer int) (<-chan TaskEvent, func())
}
