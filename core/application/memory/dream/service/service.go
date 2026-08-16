package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	memorycatalogapi "myai/core/application/memory/catalog/api"
	memorycatalogcommand "myai/core/application/memory/catalog/command"
	"myai/core/application/memory/dream/api"
	memorydreamcommand "myai/core/application/memory/dream/command"
	"myai/core/application/memory/dream/result"
	domainmemory "myai/core/domain/memory"
	memoryport "myai/core/port/memory"
)

const (
	defaultCandidateLimit = 20
	defaultMemoryLimit    = 50
)

type Service struct {
	Store        memoryport.Store
	Catalog      memorycatalogapi.Service
	Consolidator memoryport.DreamConsolidator
	IDs          memoryport.IDGenerator
	Now          func() time.Time

	mu sync.Mutex
}

var _ api.Service = (*Service)(nil)

func (service *Service) Run(ctx context.Context, command memorydreamcommand.Run) (result.Run, error) {
	if err := service.validate(); err != nil {
		return result.Run{}, err
	}
	service.mu.Lock()
	defer service.mu.Unlock()

	startedAt := service.now()
	run := domainmemory.DreamRun{
		ID: service.IDs.NewID(), Status: domainmemory.DreamRunning,
		Trigger: normalizeTrigger(command.Trigger), StartedAt: startedAt,
	}
	if err := service.Store.SaveDreamRun(ctx, run); err != nil {
		return result.Run{}, err
	}
	candidates, err := service.Store.ListCandidates(ctx, memoryport.CandidateFilter{
		Statuses: []domainmemory.CandidateStatus{domainmemory.CandidatePending},
		Limit:    normalizeLimit(command.CandidateLimit, defaultCandidateLimit),
	})
	if err != nil {
		return service.fail(ctx, run, err)
	}
	run.CandidateCount = len(candidates)
	if err := service.Store.SaveDreamRun(ctx, run); err != nil {
		return service.fail(ctx, run, err)
	}
	if len(candidates) == 0 {
		service.finish(&run, domainmemory.DreamSucceeded, "")
		if err := service.Store.SaveDreamRun(ctx, run); err != nil {
			return result.Run{DreamRun: run}, err
		}
		return result.Run{DreamRun: run}, nil
	}

	memories, err := service.Store.List(ctx, memoryport.ListFilter{
		Statuses: []domainmemory.Status{domainmemory.StatusActive},
		Limit:    normalizeLimit(command.MemoryLimit, defaultMemoryLimit),
	})
	if err != nil {
		return service.fail(ctx, run, err)
	}
	actions, err := service.Consolidator.Consolidate(ctx, candidates, memories)
	if err != nil {
		return service.fail(ctx, run, err)
	}

	candidateByID := make(map[string]domainmemory.Candidate, len(candidates))
	for _, candidate := range candidates {
		candidateByID[candidate.ID] = candidate
	}
	memoryByID := make(map[string]domainmemory.Memory, len(memories))
	for _, memory := range memories {
		memoryByID[memory.ID] = memory
	}
	seenCandidates := make(map[string]struct{}, len(actions))
	run.Actions = make([]domainmemory.DreamAction, 0, len(actions))
	for _, proposed := range actions {
		action := normalizeAction(proposed)
		if _, duplicated := seenCandidates[action.CandidateID]; duplicated {
			action.FailureReason = "candidate appears in more than one dream action"
			run.Actions = append(run.Actions, action)
			continue
		}
		seenCandidates[action.CandidateID] = struct{}{}
		candidate, exists := candidateByID[action.CandidateID]
		if !exists {
			action.FailureReason = "candidate is not part of this dream run"
			run.Actions = append(run.Actions, action)
			continue
		}
		action.CandidateTitle = candidate.Title
		service.applyAction(ctx, &run, &action, candidate, memoryByID)
		run.Actions = append(run.Actions, action)
	}
	for _, candidate := range candidates {
		if _, exists := seenCandidates[candidate.ID]; exists {
			continue
		}
		run.Actions = append(run.Actions, domainmemory.DreamAction{
			CandidateID: candidate.ID, CandidateTitle: candidate.Title, Decision: domainmemory.DecisionNeedsReview,
			Reason:        "the consolidator returned no action for this candidate",
			FailureReason: "candidate was left pending for manual review",
		})
	}

	service.finish(&run, domainmemory.DreamSucceeded, "")
	if err := service.Store.SaveDreamRun(ctx, run); err != nil {
		return result.Run{DreamRun: run}, err
	}
	return result.Run{DreamRun: run}, nil
}

func (service *Service) Get(ctx context.Context, command memorydreamcommand.Get) (result.Run, error) {
	if err := service.validate(); err != nil {
		return result.Run{}, err
	}
	run, err := service.Store.GetDreamRun(ctx, strings.TrimSpace(command.RunID))
	return result.Run{DreamRun: run}, err
}

func (service *Service) List(ctx context.Context, command memorydreamcommand.List) (result.List, error) {
	if err := service.validate(); err != nil {
		return result.List{}, err
	}
	runs, err := service.Store.ListDreamRuns(ctx, command.Limit)
	return result.List{Runs: runs}, err
}

func (service *Service) applyAction(ctx context.Context, run *domainmemory.DreamRun, action *domainmemory.DreamAction, candidate domainmemory.Candidate, memories map[string]domainmemory.Memory) {
	switch action.Decision {
	case domainmemory.DecisionCreate, domainmemory.DecisionKeepBoth:
		created, err := service.Catalog.ApproveCandidate(ctx, memorycatalogcommand.ApproveCandidate{CandidateID: candidate.ID})
		if err != nil {
			action.FailureReason = err.Error()
			return
		}
		action.MemoryID = created.Memory.ID
		action.MemoryTitle = created.Memory.Title
		action.Applied = true
		run.CreatedCount++
	case domainmemory.DecisionMerge:
		target, exists := memories[action.MemoryID]
		if !exists {
			action.FailureReason = "merge target is not an active memory in this dream run"
			return
		}
		if target.HumanLocked {
			action.FailureReason = "merge target is protected by a human lock"
			return
		}
		action.MemoryTitle = target.Title
		updated, err := service.Catalog.ApproveCandidate(ctx, memorycatalogcommand.ApproveCandidate{
			CandidateID: candidate.ID, MemoryID: target.ID, ExpectedMemoryVersion: target.CurrentVersion,
		})
		if err != nil {
			action.FailureReason = err.Error()
			return
		}
		memories[target.ID] = updated.Memory
		action.Applied = true
		run.MergedCount++
	case domainmemory.DecisionReject:
		if _, err := service.Catalog.RejectCandidate(ctx, memorycatalogcommand.RejectCandidate{
			CandidateID: candidate.ID, Note: "dream: " + action.Reason,
		}); err != nil {
			action.FailureReason = err.Error()
			return
		}
		action.Applied = true
		run.RejectedCount++
	case domainmemory.DecisionNeedsReview:
		// Keep the candidate pending for manual review.
	case domainmemory.DecisionSupersede:
		action.FailureReason = "automatic supersede is disabled until multi-memory atomic persistence is available"
	default:
		action.FailureReason = fmt.Sprintf("unsupported dream decision %q", action.Decision)
	}
}

func (service *Service) fail(ctx context.Context, run domainmemory.DreamRun, cause error) (result.Run, error) {
	service.finish(&run, domainmemory.DreamFailed, cause.Error())
	if saveErr := service.Store.SaveDreamRun(ctx, run); saveErr != nil {
		return result.Run{DreamRun: run}, errors.Join(cause, saveErr)
	}
	return result.Run{DreamRun: run}, cause
}

func (service *Service) finish(run *domainmemory.DreamRun, status domainmemory.DreamStatus, lastError string) {
	finishedAt := service.now()
	run.Status = status
	run.LastError = strings.TrimSpace(lastError)
	run.FinishedAt = &finishedAt
}

func (service *Service) validate() error {
	if service == nil || service.Store == nil {
		return errors.New("dream memory store is nil")
	}
	if service.Catalog == nil {
		return errors.New("dream memory catalog is nil")
	}
	if service.Consolidator == nil {
		return errors.New("dream consolidator is nil")
	}
	if service.IDs == nil {
		return errors.New("dream id generator is nil")
	}
	return nil
}

func (service *Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func normalizeTrigger(trigger string) string {
	trigger = strings.TrimSpace(trigger)
	if trigger == "" {
		return "manual"
	}
	return trigger
}

func normalizeLimit(value int, fallback int) int {
	if value <= 0 {
		return fallback
	}
	if value > 100 {
		return 100
	}
	return value
}

func normalizeAction(action domainmemory.DreamAction) domainmemory.DreamAction {
	action.CandidateID = strings.TrimSpace(action.CandidateID)
	action.MemoryID = strings.TrimSpace(action.MemoryID)
	action.Reason = strings.TrimSpace(action.Reason)
	action.Applied = false
	action.FailureReason = ""
	return action
}
