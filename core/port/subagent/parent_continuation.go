package subagent

import "context"

type ParentContinuation interface {
	Continue(ctx context.Context, request ParentContinuationRequest) (ParentContinuationResult, error)
}
