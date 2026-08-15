package memory

import (
	"context"

	domainagentrun "myai/core/domain/agentrun"
	domainmemory "myai/core/domain/memory"
)

type CandidateExtractor interface {
	Version() string
	Extract(ctx context.Context, run domainagentrun.Run, events []domainagentrun.Event) ([]domainmemory.CandidateDraft, error)
}

type DreamConsolidator interface {
	Consolidate(ctx context.Context, candidates []domainmemory.Candidate, existing []domainmemory.Memory) ([]domainmemory.DreamAction, error)
}
