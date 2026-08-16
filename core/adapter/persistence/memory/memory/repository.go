package memory

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	domainmemory "myai/core/domain/memory"
	memoryport "myai/core/port/memory"
)

type Repository struct {
	mu         sync.RWMutex
	memories   map[string]domainmemory.Memory
	candidates map[string]domainmemory.Candidate
	jobs       map[string]domainmemory.ExtractionJob
	dreamRuns  map[string]domainmemory.DreamRun
}

var _ memoryport.Store = (*Repository)(nil)

func New() *Repository {
	return &Repository{
		memories:   make(map[string]domainmemory.Memory),
		candidates: make(map[string]domainmemory.Candidate),
		jobs:       make(map[string]domainmemory.ExtractionJob),
		dreamRuns:  make(map[string]domainmemory.DreamRun),
	}
}

func (repository *Repository) Get(_ context.Context, memoryID string) (domainmemory.Memory, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	memory, ok := repository.memories[strings.TrimSpace(memoryID)]
	if !ok {
		return domainmemory.Memory{}, memoryport.ErrNotFound
	}
	return cloneMemory(memory), nil
}

func (repository *Repository) List(_ context.Context, filter memoryport.ListFilter) ([]domainmemory.Memory, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	items := make([]domainmemory.Memory, 0, len(repository.memories))
	for _, memory := range repository.memories {
		if !filterMemory(memory, filter) {
			continue
		}
		items = append(items, cloneMemory(memory))
	}
	sort.Slice(items, func(left, right int) bool { return items[left].UpdatedAt.After(items[right].UpdatedAt) })
	if filter.Limit > 0 && len(items) > filter.Limit {
		items = items[:filter.Limit]
	}
	return items, nil
}

func (repository *Repository) Save(_ context.Context, memory domainmemory.Memory) error {
	if err := memory.Validate(); err != nil {
		return err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.ensureMaps()
	repository.memories[memory.ID] = cloneMemory(memory)
	return nil
}

func (repository *Repository) GetCandidate(_ context.Context, candidateID string) (domainmemory.Candidate, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	candidate, ok := repository.candidates[strings.TrimSpace(candidateID)]
	if !ok {
		return domainmemory.Candidate{}, memoryport.ErrNotFound
	}
	return cloneCandidate(candidate), nil
}

func (repository *Repository) ListCandidates(_ context.Context, filter memoryport.CandidateFilter) ([]domainmemory.Candidate, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	items := make([]domainmemory.Candidate, 0, len(repository.candidates))
	for _, candidate := range repository.candidates {
		if len(filter.Statuses) > 0 && !containsCandidateStatus(filter.Statuses, candidate.Status) {
			continue
		}
		items = append(items, cloneCandidate(candidate))
	}
	sort.Slice(items, func(left, right int) bool { return items[left].UpdatedAt.After(items[right].UpdatedAt) })
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (repository *Repository) SaveCandidate(_ context.Context, candidate domainmemory.Candidate) error {
	if err := candidate.Validate(); err != nil {
		return err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.ensureMaps()
	repository.candidates[candidate.ID] = cloneCandidate(candidate)
	return nil
}

func (repository *Repository) SaveCandidateApproval(_ context.Context, memory domainmemory.Memory, candidate domainmemory.Candidate, expectedMemoryVersion int) error {
	if err := memory.Validate(); err != nil {
		return err
	}
	if err := candidate.Validate(); err != nil {
		return err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.ensureMaps()
	current, exists := repository.candidates[candidate.ID]
	if !exists {
		return memoryport.ErrNotFound
	}
	if current.Status != domainmemory.CandidatePending {
		return memoryport.ErrConflict
	}
	if expectedMemoryVersion > 0 {
		currentMemory, exists := repository.memories[memory.ID]
		if !exists {
			return memoryport.ErrNotFound
		}
		if currentMemory.CurrentVersion != expectedMemoryVersion {
			return memoryport.ErrConflict
		}
	}
	repository.memories[memory.ID] = cloneMemory(memory)
	repository.candidates[candidate.ID] = cloneCandidate(candidate)
	return nil
}

func (repository *Repository) SaveCandidateRejection(_ context.Context, candidate domainmemory.Candidate) error {
	if err := candidate.Validate(); err != nil {
		return err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.ensureMaps()
	current, exists := repository.candidates[candidate.ID]
	if !exists {
		return memoryport.ErrNotFound
	}
	if current.Status != domainmemory.CandidatePending {
		return memoryport.ErrConflict
	}
	repository.candidates[candidate.ID] = cloneCandidate(candidate)
	return nil
}

func (repository *Repository) RecordUse(_ context.Context, memoryID string, usedAt time.Time) error {
	if usedAt.IsZero() {
		return errors.New("memory used_at is empty")
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	memory, exists := repository.memories[strings.TrimSpace(memoryID)]
	if !exists {
		return memoryport.ErrNotFound
	}
	if memory.Status != domainmemory.StatusActive {
		return nil
	}
	usedAt = usedAt.UTC()
	memory.UseCount++
	memory.LastUsedAt = &usedAt
	memory.UpdatedAt = usedAt
	repository.memories[memory.ID] = cloneMemory(memory)
	return nil
}

func (repository *Repository) GetExtractionJob(_ context.Context, jobID string) (domainmemory.ExtractionJob, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	job, ok := repository.jobs[strings.TrimSpace(jobID)]
	if !ok {
		return domainmemory.ExtractionJob{}, memoryport.ErrNotFound
	}
	return job, nil
}

func (repository *Repository) GetExtractionJobByRun(_ context.Context, agentRunID string, extractorVersion string) (domainmemory.ExtractionJob, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	for _, job := range repository.jobs {
		if job.AgentRunID == strings.TrimSpace(agentRunID) && job.ExtractorVersion == strings.TrimSpace(extractorVersion) {
			return job, nil
		}
	}
	return domainmemory.ExtractionJob{}, memoryport.ErrNotFound
}

func (repository *Repository) ListExtractionJobs(_ context.Context, statuses []domainmemory.JobStatus, limit int) ([]domainmemory.ExtractionJob, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	items := make([]domainmemory.ExtractionJob, 0, len(repository.jobs))
	for _, job := range repository.jobs {
		if len(statuses) > 0 && !containsJobStatus(statuses, job.Status) {
			continue
		}
		items = append(items, job)
	}
	sort.Slice(items, func(left, right int) bool { return items[left].UpdatedAt.Before(items[right].UpdatedAt) })
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (repository *Repository) SaveExtractionJob(_ context.Context, job domainmemory.ExtractionJob) error {
	if err := job.Validate(); err != nil {
		return err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.ensureMaps()
	for existingID, existing := range repository.jobs {
		if existingID != job.ID && existing.AgentRunID == job.AgentRunID && existing.ExtractorVersion == job.ExtractorVersion {
			return memoryport.ErrConflict
		}
	}
	repository.jobs[job.ID] = job
	return nil
}

func (repository *Repository) GetDreamRun(_ context.Context, runID string) (domainmemory.DreamRun, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	run, ok := repository.dreamRuns[strings.TrimSpace(runID)]
	if !ok {
		return domainmemory.DreamRun{}, memoryport.ErrNotFound
	}
	return cloneDreamRun(run), nil
}

func (repository *Repository) ListDreamRuns(_ context.Context, limit int) ([]domainmemory.DreamRun, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	items := make([]domainmemory.DreamRun, 0, len(repository.dreamRuns))
	for _, run := range repository.dreamRuns {
		items = append(items, cloneDreamRun(run))
	}
	sort.Slice(items, func(left, right int) bool { return items[left].StartedAt.After(items[right].StartedAt) })
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (repository *Repository) SaveDreamRun(_ context.Context, run domainmemory.DreamRun) error {
	if err := run.Validate(); err != nil {
		return err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.ensureMaps()
	repository.dreamRuns[run.ID] = cloneDreamRun(run)
	return nil
}

func filterMemory(memory domainmemory.Memory, filter memoryport.ListFilter) bool {
	if !filter.IncludeDeleted && memory.Status == domainmemory.StatusDeleted {
		return false
	}
	if len(filter.Kinds) > 0 && !containsKind(filter.Kinds, memory.Kind) {
		return false
	}
	if len(filter.Statuses) > 0 && !containsStatus(filter.Statuses, memory.Status) {
		return false
	}
	if len(filter.ScopeTypes) > 0 && !containsScopeType(filter.ScopeTypes, memory.Scope.Type) {
		return false
	}
	if filter.ScopeKey != "" && memory.Scope.Key != filter.ScopeKey {
		return false
	}
	for _, tag := range filter.Tags {
		if !containsString(memory.Tags, strings.ToLower(strings.TrimSpace(tag))) {
			return false
		}
	}
	if strings.TrimSpace(filter.Text) != "" {
		query := strings.ToLower(strings.TrimSpace(filter.Text))
		revision, _ := memory.CurrentRevision()
		content := revision.Content
		text := strings.ToLower(strings.Join([]string{memory.Title, content.Goal, content.ApplicableContext, content.Approach, content.Result, content.PainPoints, content.RootCause, content.Lessons, content.Verification}, "\n"))
		if !strings.Contains(text, query) {
			return false
		}
	}
	return true
}

func (repository *Repository) ensureMaps() {
	if repository.memories == nil {
		repository.memories = make(map[string]domainmemory.Memory)
	}
	if repository.candidates == nil {
		repository.candidates = make(map[string]domainmemory.Candidate)
	}
	if repository.jobs == nil {
		repository.jobs = make(map[string]domainmemory.ExtractionJob)
	}
	if repository.dreamRuns == nil {
		repository.dreamRuns = make(map[string]domainmemory.DreamRun)
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func containsKind(items []domainmemory.Kind, target domainmemory.Kind) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func containsStatus(items []domainmemory.Status, target domainmemory.Status) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func containsScopeType(items []domainmemory.ScopeType, target domainmemory.ScopeType) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func containsCandidateStatus(items []domainmemory.CandidateStatus, target domainmemory.CandidateStatus) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func containsJobStatus(items []domainmemory.JobStatus, target domainmemory.JobStatus) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func cloneMemory(memory domainmemory.Memory) domainmemory.Memory {
	memory.Tags = append([]string(nil), memory.Tags...)
	memory.Revisions = append([]domainmemory.Revision(nil), memory.Revisions...)
	for index := range memory.Revisions {
		memory.Revisions[index].Sources = append([]domainmemory.SourceRef(nil), memory.Revisions[index].Sources...)
		for sourceIndex := range memory.Revisions[index].Sources {
			memory.Revisions[index].Sources[sourceIndex].EventIDs = append([]string(nil), memory.Revisions[index].Sources[sourceIndex].EventIDs...)
		}
	}
	if memory.LastUsedAt != nil {
		value := *memory.LastUsedAt
		memory.LastUsedAt = &value
	}
	if memory.DeletedAt != nil {
		value := *memory.DeletedAt
		memory.DeletedAt = &value
	}
	return memory
}

func cloneCandidate(candidate domainmemory.Candidate) domainmemory.Candidate {
	candidate.Tags = append([]string(nil), candidate.Tags...)
	candidate.Sources = append([]domainmemory.SourceRef(nil), candidate.Sources...)
	for index := range candidate.Sources {
		candidate.Sources[index].EventIDs = append([]string(nil), candidate.Sources[index].EventIDs...)
	}
	return candidate
}

func cloneDreamRun(run domainmemory.DreamRun) domainmemory.DreamRun {
	run.Actions = append([]domainmemory.DreamAction(nil), run.Actions...)
	if run.FinishedAt != nil {
		value := *run.FinishedAt
		run.FinishedAt = &value
	}
	return run
}
