package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	domainsubagent "myai/core/domain/subagent"
	subagentport "myai/core/port/subagent"
)

type Repository struct {
	mu          sync.RWMutex
	definitions map[string]domainsubagent.Definition
	tasks       map[string]domainsubagent.Task
	runs        map[string][]domainsubagent.Run
}

var _ subagentport.DefinitionRepository = (*Repository)(nil)
var _ subagentport.TaskRepository = (*Repository)(nil)
var _ subagentport.RunRepository = (*Repository)(nil)
var _ subagentport.TaskRunRepository = (*Repository)(nil)
var _ subagentport.InterruptedTaskRepository = (*Repository)(nil)

func NewRepository() *Repository {
	return &Repository{
		definitions: make(map[string]domainsubagent.Definition),
		tasks:       make(map[string]domainsubagent.Task),
		runs:        make(map[string][]domainsubagent.Run),
	}
}

func (repository *Repository) SaveDefinition(_ context.Context, definition domainsubagent.Definition) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.definitions[definition.ID] = domainsubagent.CloneDefinition(definition)
	return nil
}

func (repository *Repository) GetDefinition(_ context.Context, definitionID string) (domainsubagent.Definition, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	definition, ok := repository.definitions[strings.TrimSpace(definitionID)]
	if !ok {
		return domainsubagent.Definition{}, subagentport.ErrNotFound
	}
	return domainsubagent.CloneDefinition(definition), nil
}

func (repository *Repository) ListDefinitions(_ context.Context, includeDeleted bool) ([]domainsubagent.Definition, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	items := make([]domainsubagent.Definition, 0, len(repository.definitions))
	for _, definition := range repository.definitions {
		if definition.Deleted != includeDeleted && definition.Deleted {
			continue
		}
		items = append(items, domainsubagent.CloneDefinition(definition))
	}
	return items, nil
}

func (repository *Repository) DeleteDefinition(_ context.Context, definitionID string) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	definition, ok := repository.definitions[strings.TrimSpace(definitionID)]
	if !ok {
		return subagentport.ErrNotFound
	}
	now := time.Now().UTC()
	definition.Deleted = true
	definition.DeletedAt = &now
	definition.UpdatedAt = now
	repository.definitions[definition.ID] = definition
	return nil
}

func (repository *Repository) SaveTask(_ context.Context, task domainsubagent.Task) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.tasks[task.ID] = domainsubagent.CloneTask(task)
	return nil
}

func (repository *Repository) GetTask(_ context.Context, taskID string) (domainsubagent.Task, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	task, ok := repository.tasks[strings.TrimSpace(taskID)]
	if !ok {
		return domainsubagent.Task{}, subagentport.ErrNotFound
	}
	return domainsubagent.CloneTask(task), nil
}

func (repository *Repository) ListTasks(_ context.Context, parentSessionID string, limit int) ([]domainsubagent.Task, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	items := make([]domainsubagent.Task, 0)
	for _, task := range repository.tasks {
		if parentSessionID != "" && task.ParentSessionID != parentSessionID {
			continue
		}
		items = append(items, domainsubagent.CloneTask(task))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (repository *Repository) ListNonTerminalTasks(_ context.Context) ([]domainsubagent.Task, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	items := make([]domainsubagent.Task, 0)
	for _, task := range repository.tasks {
		if task.Terminal() {
			continue
		}
		items = append(items, domainsubagent.CloneTask(task))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	return items, nil
}

func (repository *Repository) SaveTaskAndRun(_ context.Context, task domainsubagent.Task, run domainsubagent.Run) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.tasks[task.ID] = domainsubagent.CloneTask(task)
	items := repository.runs[run.TaskID]
	for index := range items {
		if items[index].ID == run.ID {
			items[index] = domainsubagent.CloneRun(run)
			repository.runs[run.TaskID] = items
			return nil
		}
	}
	repository.runs[run.TaskID] = append(items, domainsubagent.CloneRun(run))
	return nil
}

func (repository *Repository) SaveRun(_ context.Context, run domainsubagent.Run) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	items := repository.runs[run.TaskID]
	for index := range items {
		if items[index].ID == run.ID {
			items[index] = domainsubagent.CloneRun(run)
			repository.runs[run.TaskID] = items
			return nil
		}
	}
	repository.runs[run.TaskID] = append(items, domainsubagent.CloneRun(run))
	return nil
}

func (repository *Repository) ListRuns(_ context.Context, taskID string) ([]domainsubagent.Run, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	items := repository.runs[strings.TrimSpace(taskID)]
	result := make([]domainsubagent.Run, len(items))
	for index, run := range items {
		result[index] = domainsubagent.CloneRun(run)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Sequence < result[j].Sequence })
	return result, nil
}
