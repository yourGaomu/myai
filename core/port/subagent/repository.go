package subagent

import (
	"context"
	"errors"

	domainsubagent "myai/core/domain/subagent"
)

var ErrNotFound = errors.New("subagent record not found")

type DefinitionRepository interface {
	SaveDefinition(ctx context.Context, definition domainsubagent.Definition) error
	GetDefinition(ctx context.Context, definitionID string) (domainsubagent.Definition, error)
	ListDefinitions(ctx context.Context, includeDeleted bool) ([]domainsubagent.Definition, error)
	DeleteDefinition(ctx context.Context, definitionID string) error
}

type TaskRepository interface {
	SaveTask(ctx context.Context, task domainsubagent.Task) error
	GetTask(ctx context.Context, taskID string) (domainsubagent.Task, error)
	ListTasks(ctx context.Context, parentSessionID string, limit int) ([]domainsubagent.Task, error)
}

// TaskRunRepository persists one task transition and its current run as a
// single unit. Adapters must not leave only one side of the transition saved.
type TaskRunRepository interface {
	SaveTaskAndRun(ctx context.Context, task domainsubagent.Task, run domainsubagent.Run) error
}

// InterruptedTaskRepository exposes the non-terminal tasks that need to be
// reconciled after an unclean process shutdown.
type InterruptedTaskRepository interface {
	ListNonTerminalTasks(ctx context.Context) ([]domainsubagent.Task, error)
}

type RunRepository interface {
	SaveRun(ctx context.Context, run domainsubagent.Run) error
	ListRuns(ctx context.Context, taskID string) ([]domainsubagent.Run, error)
}
