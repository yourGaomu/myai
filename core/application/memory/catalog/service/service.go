package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"myai/core/application/memory/catalog/api"
	"myai/core/application/memory/catalog/command"
	"myai/core/application/memory/catalog/result"
	domainmemory "myai/core/domain/memory"
	memoryport "myai/core/port/memory"
)

type CatalogService struct {
	Store memoryport.Store
	IDs   memoryport.IDGenerator
	Now   func() time.Time
}

var _ api.Service = CatalogService{}

func (service CatalogService) List(ctx context.Context, command command.List) (result.List, error) {
	if err := service.validate(); err != nil {
		return result.List{}, err
	}
	memories, err := service.Store.List(ctx, normalizeListFilter(command.Filter))
	if err != nil {
		return result.List{}, err
	}
	return result.List{Memories: memories}, nil
}

func (service CatalogService) Get(ctx context.Context, command command.Get) (result.Detail, error) {
	if err := service.validate(); err != nil {
		return result.Detail{}, err
	}
	memory, err := service.Store.Get(ctx, strings.TrimSpace(command.MemoryID))
	if err != nil {
		return result.Detail{}, err
	}
	return result.Detail{Memory: memory}, nil
}

func (service CatalogService) Create(ctx context.Context, command command.Create) (result.Detail, error) {
	if err := service.validate(); err != nil {
		return result.Detail{}, err
	}
	now := service.now()
	kind := command.Kind
	if kind == "" {
		kind = domainmemory.KindExperience
	}
	scope := normalizeScope(command.Scope)
	revision := domainmemory.Revision{
		ID: service.IDs.NewID(), Version: 1, Content: command.Content,
		Confidence: normalizeConfidence(command.Confidence, 1), Author: domainmemory.AuthorHuman,
		Sources: []domainmemory.SourceRef{{Type: domainmemory.SourceManual, CreatedAt: now}}, CreatedAt: now,
	}
	memory := domainmemory.Memory{
		ID: service.IDs.NewID(), Title: strings.TrimSpace(command.Title), Kind: kind, Scope: scope,
		Tags: normalizeTags(command.Tags), Status: domainmemory.StatusActive, CurrentVersion: 1,
		Revisions: []domainmemory.Revision{revision}, HumanLocked: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := memory.Validate(); err != nil {
		return result.Detail{}, err
	}
	if err := service.Store.Save(ctx, memory); err != nil {
		return result.Detail{}, err
	}
	return result.Detail{Memory: memory}, nil
}

func (service CatalogService) Update(ctx context.Context, command command.Update) (result.Detail, error) {
	if err := service.validate(); err != nil {
		return result.Detail{}, err
	}
	memory, err := service.Store.Get(ctx, strings.TrimSpace(command.MemoryID))
	if err != nil {
		return result.Detail{}, err
	}
	if memory.Status == domainmemory.StatusDeleted {
		return result.Detail{}, errors.New("deleted memory must be restored before editing")
	}
	now := service.now()
	memory.Title = strings.TrimSpace(command.Title)
	if command.Kind != "" {
		memory.Kind = command.Kind
	}
	if command.Scope.Type != "" {
		memory.Scope = normalizeScope(command.Scope)
	}
	memory.Tags = normalizeTags(command.Tags)
	if err := memory.AppendRevision(domainmemory.Revision{
		ID: service.IDs.NewID(), Content: command.Content,
		Confidence: normalizeConfidence(command.Confidence, 1), Author: domainmemory.AuthorHuman,
		Sources: []domainmemory.SourceRef{{Type: domainmemory.SourceManual, CreatedAt: now}}, CreatedAt: now,
	}, now); err != nil {
		return result.Detail{}, err
	}
	memory.Status = domainmemory.StatusActive
	memory.HumanLocked = true
	if err := service.Store.Save(ctx, memory); err != nil {
		return result.Detail{}, err
	}
	return result.Detail{Memory: memory}, nil
}

func (service CatalogService) Delete(ctx context.Context, command command.Delete) error {
	if err := service.validate(); err != nil {
		return err
	}
	memory, err := service.Store.Get(ctx, strings.TrimSpace(command.MemoryID))
	if err != nil {
		return err
	}
	if err := memory.MarkDeleted(service.now(), command.Reason); err != nil {
		return err
	}
	return service.Store.Save(ctx, memory)
}

func (service CatalogService) Restore(ctx context.Context, command command.Restore) (result.Detail, error) {
	if err := service.validate(); err != nil {
		return result.Detail{}, err
	}
	memory, err := service.Store.Get(ctx, strings.TrimSpace(command.MemoryID))
	if err != nil {
		return result.Detail{}, err
	}
	if err := memory.Restore(service.now()); err != nil {
		return result.Detail{}, err
	}
	if err := service.Store.Save(ctx, memory); err != nil {
		return result.Detail{}, err
	}
	return result.Detail{Memory: memory}, nil
}

func (service CatalogService) CreateCandidate(ctx context.Context, command command.CreateCandidate) (result.Candidate, error) {
	if err := service.validate(); err != nil {
		return result.Candidate{}, err
	}
	now := service.now()
	candidate := domainmemory.Candidate{
		ID: service.IDs.NewID(), Title: strings.TrimSpace(command.Title), Kind: command.Kind,
		Scope: normalizeScope(command.Scope), Tags: normalizeTags(command.Tags), Content: command.Content,
		Confidence: normalizeConfidence(command.Confidence, 0.5), Sources: append([]domainmemory.SourceRef(nil), command.Sources...),
		Status: domainmemory.CandidatePending, CreatedAt: now, UpdatedAt: now,
	}
	if err := candidate.Validate(); err != nil {
		return result.Candidate{}, err
	}
	if err := service.Store.SaveCandidate(ctx, candidate); err != nil {
		return result.Candidate{}, err
	}
	return result.Candidate{MemoryCandidate: candidate}, nil
}

func (service CatalogService) ListCandidates(ctx context.Context, command command.ListCandidates) (result.Candidates, error) {
	if err := service.validate(); err != nil {
		return result.Candidates{}, err
	}
	items, err := service.Store.ListCandidates(ctx, command.Filter)
	if err != nil {
		return result.Candidates{}, err
	}
	return result.Candidates{Items: items}, nil
}

func (service CatalogService) ApproveCandidate(ctx context.Context, command command.ApproveCandidate) (result.Detail, error) {
	if err := service.validate(); err != nil {
		return result.Detail{}, err
	}
	candidate, err := service.Store.GetCandidate(ctx, strings.TrimSpace(command.CandidateID))
	if err != nil {
		return result.Detail{}, err
	}
	if candidate.Status != domainmemory.CandidatePending {
		return result.Detail{}, fmt.Errorf("memory candidate is already %s", candidate.Status)
	}
	now := service.now()
	memoryID := strings.TrimSpace(command.MemoryID)
	merging := memoryID != ""
	var memory domainmemory.Memory
	if merging {
		memory, err = service.Store.Get(ctx, memoryID)
		if err != nil {
			return result.Detail{}, err
		}
		if memory.Status == domainmemory.StatusDeleted {
			return result.Detail{}, errors.New("cannot approve candidate into deleted memory")
		}
		if command.ExpectedMemoryVersion > 0 && memory.CurrentVersion != command.ExpectedMemoryVersion {
			return result.Detail{}, fmt.Errorf(
				"memory version changed: expected %d, got %d",
				command.ExpectedMemoryVersion, memory.CurrentVersion,
			)
		}
		if !command.HumanApprove && memory.HumanLocked {
			return result.Detail{}, errors.New("cannot automatically merge candidate into human-locked memory")
		}
		if err := memory.AppendRevision(domainmemory.Revision{
			ID: service.IDs.NewID(), Content: candidate.Content, Confidence: candidate.Confidence,
			Author: domainmemory.AuthorModel, Sources: candidate.Sources, CreatedAt: now,
		}, now); err != nil {
			return result.Detail{}, err
		}
		memory.Tags = normalizeTags(candidate.Tags)
		memory.Status = domainmemory.StatusActive
		memory.HumanLocked = command.HumanApprove
	} else {
		memoryID = service.IDs.NewID()
		revision := domainmemory.Revision{
			ID: service.IDs.NewID(), Version: 1, Content: candidate.Content, Confidence: candidate.Confidence,
			Author: domainmemory.AuthorModel, Sources: candidate.Sources, CreatedAt: now,
		}
		memory = domainmemory.Memory{
			ID: memoryID, Title: candidate.Title, Kind: candidate.Kind, Scope: candidate.Scope,
			Tags: normalizeTags(candidate.Tags), Status: domainmemory.StatusActive, CurrentVersion: 1,
			Revisions: []domainmemory.Revision{revision}, HumanLocked: command.HumanApprove,
			CreatedAt: now, UpdatedAt: now,
		}
	}
	if err := memory.Validate(); err != nil {
		return result.Detail{}, err
	}
	candidate.Status = domainmemory.CandidateApproved
	if merging && !command.HumanApprove {
		candidate.Status = domainmemory.CandidateMerged
	}
	candidate.TargetMemoryID = memory.ID
	candidate.UpdatedAt = now
	if err := service.Store.SaveCandidateApproval(ctx, memory, candidate, command.ExpectedMemoryVersion); err != nil {
		return result.Detail{}, err
	}
	return result.Detail{Memory: memory}, nil
}

func (service CatalogService) RejectCandidate(ctx context.Context, command command.RejectCandidate) (result.Candidate, error) {
	if err := service.validate(); err != nil {
		return result.Candidate{}, err
	}
	candidate, err := service.Store.GetCandidate(ctx, strings.TrimSpace(command.CandidateID))
	if err != nil {
		return result.Candidate{}, err
	}
	if candidate.Status != domainmemory.CandidatePending {
		return result.Candidate{}, fmt.Errorf("memory candidate is already %s", candidate.Status)
	}
	candidate.Status = domainmemory.CandidateRejected
	candidate.ReviewNote = strings.TrimSpace(command.Note)
	candidate.UpdatedAt = service.now()
	if err := service.Store.SaveCandidateRejection(ctx, candidate); err != nil {
		return result.Candidate{}, err
	}
	return result.Candidate{MemoryCandidate: candidate}, nil
}

func (service CatalogService) RecordUse(ctx context.Context, command command.RecordUse) error {
	if err := service.validate(); err != nil {
		return err
	}
	return service.Store.RecordUse(ctx, strings.TrimSpace(command.MemoryID), service.now())
}

func (service CatalogService) validate() error {
	if service.Store == nil {
		return errors.New("memory store is nil")
	}
	if service.IDs == nil {
		return errors.New("memory id generator is nil")
	}
	return nil
}

func (service CatalogService) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func normalizeScope(scope domainmemory.Scope) domainmemory.Scope {
	if scope.Type == "" {
		return domainmemory.Scope{Type: domainmemory.ScopeGlobal}
	}
	scope.Key = strings.TrimSpace(scope.Key)
	return scope
}

func normalizeTags(tags []string) []string {
	result := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	return result
}

func normalizeConfidence(value float64, fallback float64) float64 {
	if value == 0 {
		return fallback
	}
	return value
}

func normalizeListFilter(filter memoryport.ListFilter) memoryport.ListFilter {
	filter.Text = strings.TrimSpace(filter.Text)
	filter.Tags = normalizeTags(filter.Tags)
	filter.ScopeKey = strings.TrimSpace(filter.ScopeKey)
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}
	return filter
}
