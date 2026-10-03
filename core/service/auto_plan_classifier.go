package service

import (
	"context"
	"myai/core/session"
)

// AutoPlanIntent describes the kind of work requested by the current user
// turn. Keeping this as a small, explicit vocabulary lets the chat facade
// make orchestration decisions without inspecting classifier internals.
type AutoPlanIntent string

const (
	AutoPlanIntentConversation   AutoPlanIntent = "conversation"
	AutoPlanIntentExplanation    AutoPlanIntent = "explanation"
	AutoPlanIntentImplementation AutoPlanIntent = "implementation"
	AutoPlanIntentResumePlan     AutoPlanIntent = "resume_plan"
)

// AutoPlanAction is the single orchestration action selected for a user turn.
// Keeping this mutually exclusive prevents an invalid combination such as
// planning a new workflow and resuming an existing workflow at the same time.
type AutoPlanAction string

const (
	AutoPlanActionChat        AutoPlanAction = "chat"
	AutoPlanActionPlanOnly    AutoPlanAction = "plan_only"
	AutoPlanActionPlanExecute AutoPlanAction = "plan_execute"
	AutoPlanActionResumePlan  AutoPlanAction = "plan_resume"
)

// AutoPlanDecision is the structured result used by ChatService before it
// appends the user turn. Action is intentionally explicit instead of being
// inferred from Intent so a classifier can refuse planning for an
// implementation-shaped request when context or permissions are missing.
type AutoPlanDecision struct {
	Intent     AutoPlanIntent
	Action     AutoPlanAction
	Confidence float64
	Reason     string
}

// AutoPlanClassifier decides whether a root user request should enter the
// autonomous plan -> execute loop. Implementations must not mutate the
// session; classification happens before the user message is appended.
type AutoPlanClassifier interface {
	Classify(ctx context.Context, current *session.Session, input string) (AutoPlanDecision, error)
}
