package workspace

import "context"

type Manager interface {
	Prepare(ctx context.Context, request PrepareRequest) (PreparedWorkspace, error)
	Collect(ctx context.Context, request CollectRequest) (CollectedChanges, error)
	Apply(ctx context.Context, request ApplyRequest) (AppliedChanges, error)
	Discard(ctx context.Context, request DiscardRequest) (DiscardedChanges, error)
}
