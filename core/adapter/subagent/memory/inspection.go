package memory

import (
	"context"
	subagentport "myai/core/port/subagent"
)

var _ subagentport.TaskEventHistoryRepository = (*Repository)(nil)

func (repository *Repository) QueryTaskEvents(_ context.Context, filter subagentport.TaskEventFilter) ([]subagentport.TaskEvent, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	limit := filter.Limit
	if limit <= 0 || limit > 201 {
		limit = 201
	}
	items := make([]subagentport.TaskEvent, 0)
	for _, event := range repository.events {
		if event.Sequence <= filter.AfterSequence || event.Sequence > filter.ThroughSequence {
			continue
		}
		if event.Task.ParentSessionID != filter.ParentSessionID {
			continue
		}
		if filter.TaskID != "" && event.Task.ID != filter.TaskID {
			continue
		}
		if filter.RunID != "" && event.RunID != filter.RunID {
			continue
		}
		items = append(items, cloneTaskEvent(event))
		if len(items) >= limit {
			break
		}
	}
	return items, nil
}
