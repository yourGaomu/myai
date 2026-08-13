package agentrun

import (
	"context"
	"sort"
	"strings"
	"sync"

	domainagentrun "myai/core/domain/agentrun"
	agentrunport "myai/core/port/agentrun"
)

type Repository struct {
	mu     sync.RWMutex
	runs   map[string]domainagentrun.Run
	events map[string][]domainagentrun.Event
}

var _ agentrunport.Repository = (*Repository)(nil)

func New() *Repository {
	return &Repository{
		runs:   make(map[string]domainagentrun.Run),
		events: make(map[string][]domainagentrun.Event),
	}
}

func (r *Repository) SaveRun(_ context.Context, run domainagentrun.Run) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureMaps()
	r.runs[run.ID] = cloneRun(run)
	return nil
}

func (r *Repository) GetRun(_ context.Context, runID string) (domainagentrun.Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	run, ok := r.runs[strings.TrimSpace(runID)]
	if !ok {
		return domainagentrun.Run{}, agentrunport.ErrNotFound
	}
	return cloneRun(run), nil
}

func (r *Repository) ListRunning(_ context.Context) ([]domainagentrun.Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]domainagentrun.Run, 0)
	for _, run := range r.runs {
		if run.Status == domainagentrun.StatusRunning {
			items = append(items, cloneRun(run))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].StartedAt.Before(items[j].StartedAt) })
	return items, nil
}

func (r *Repository) NextEventSequence(_ context.Context, runID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureMaps()
	run, ok := r.runs[strings.TrimSpace(runID)]
	if !ok {
		return 0, agentrunport.ErrNotFound
	}
	run.LastSequence++
	r.runs[run.ID] = run
	return run.LastSequence, nil
}

func (r *Repository) SaveEvent(_ context.Context, event domainagentrun.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureMaps()
	if _, ok := r.runs[event.RunID]; !ok {
		return agentrunport.ErrNotFound
	}
	r.events[event.RunID] = append(r.events[event.RunID], event)
	return nil
}

func (r *Repository) ReplaceEventContent(_ context.Context, runID string, eventID string, content string, truncated bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	runID = strings.TrimSpace(runID)
	events := r.events[runID]
	for index := range events {
		if events[index].ID == strings.TrimSpace(eventID) {
			events[index].Content = content
			events[index].Truncated = truncated
			r.events[runID] = events
			return nil
		}
	}
	return agentrunport.ErrNotFound
}

func (r *Repository) ListRuns(_ context.Context, sessionID string, limit int) ([]domainagentrun.Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]domainagentrun.Run, 0)
	for _, run := range r.runs {
		if run.SessionID == strings.TrimSpace(sessionID) {
			items = append(items, cloneRun(run))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].StartedAt.After(items[j].StartedAt) })
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if len(items) > limit {
		items = items[:limit]
	}
	sort.Slice(items, func(i, j int) bool { return items[i].StartedAt.Before(items[j].StartedAt) })
	return items, nil
}

func (r *Repository) ListEvents(_ context.Context, runIDs []string) ([]domainagentrun.Event, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]domainagentrun.Event, 0)
	for _, runID := range runIDs {
		items = append(items, r.events[runID]...)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].RunID == items[j].RunID {
			return items[i].Sequence < items[j].Sequence
		}
		return items[i].RunID < items[j].RunID
	})
	return append([]domainagentrun.Event(nil), items...), nil
}

func (r *Repository) ensureMaps() {
	if r.runs == nil {
		r.runs = make(map[string]domainagentrun.Run)
	}
	if r.events == nil {
		r.events = make(map[string][]domainagentrun.Event)
	}
}

func cloneRun(run domainagentrun.Run) domainagentrun.Run {
	if run.FinishedAt != nil {
		finishedAt := *run.FinishedAt
		run.FinishedAt = &finishedAt
	}
	return run
}
