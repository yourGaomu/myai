package api

import (
	"context"

	"myai/core/application/memory/catalog/command"
	"myai/core/application/memory/catalog/result"
)

type Service interface {
	List(ctx context.Context, command command.List) (result.List, error)
	Get(ctx context.Context, command command.Get) (result.Detail, error)
	Create(ctx context.Context, command command.Create) (result.Detail, error)
	Update(ctx context.Context, command command.Update) (result.Detail, error)
	Delete(ctx context.Context, command command.Delete) error
	Restore(ctx context.Context, command command.Restore) (result.Detail, error)
	CreateCandidate(ctx context.Context, command command.CreateCandidate) (result.Candidate, error)
	ListCandidates(ctx context.Context, command command.ListCandidates) (result.Candidates, error)
	ApproveCandidate(ctx context.Context, command command.ApproveCandidate) (result.Detail, error)
	RejectCandidate(ctx context.Context, command command.RejectCandidate) (result.Candidate, error)
	RecordUse(ctx context.Context, command command.RecordUse) error
}
