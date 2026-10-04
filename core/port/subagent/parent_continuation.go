package subagent

import (
	"context"

	domainsubagent "myai/core/domain/subagent"
	"myai/core/llm"
)

type ParentContinuation interface {
	Continue(ctx context.Context, request ParentContinuationRequest) (ParentContinuationResult, error)
}

// ParentCompletionNotifier places a structured child result into the parent
// session input queue and may request an asynchronous parent wakeup. It never
// runs the parent model synchronously on the child completion stack.
type ParentCompletionNotifier interface {
	Notify(ctx context.Context, task domainsubagent.Task) error
}

// PendingParentContinuation consumes already queued parent input and starts a
// new parent turn. Keeping this separate from ParentContinuation allows the
// runtime to avoid injecting the same child result twice.
type PendingParentContinuation interface {
	ContinuePending(ctx context.Context, sessionID string, stream llm.ChatStreamHandler) (ParentContinuationResult, error)
}
