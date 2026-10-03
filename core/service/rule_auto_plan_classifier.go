package service

import (
	"context"
	"strings"

	"myai/core/session"
)

// RuleBasedAutoPlanClassifier is the conservative compatibility classifier.
// It handles unambiguous requests without making another model call.
type RuleBasedAutoPlanClassifier struct{}

var _ AutoPlanClassifier = RuleBasedAutoPlanClassifier{}

func (RuleBasedAutoPlanClassifier) Classify(_ context.Context, current *session.Session, input string) (AutoPlanDecision, error) {
	if current != nil && current.Kind == session.KindSubagent {
		return AutoPlanDecision{
			Intent: AutoPlanIntentConversation,
			Action: AutoPlanActionChat,
			Reason: "subagent sessions do not recursively start autonomous planning",
		}, nil
	}

	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return AutoPlanDecision{Intent: AutoPlanIntentConversation, Action: AutoPlanActionChat, Reason: "empty request"}, nil
	}
	//需要继续计划的执行
	if shouldResumePlanRequest(current, trimmed) {
		return AutoPlanDecision{
			Intent:     AutoPlanIntentResumePlan,
			Action:     AutoPlanActionResumePlan,
			Confidence: 1,
			Reason:     "explicit request to resume the existing plan",
		}, nil
	}
	//
	if isImplementationQuestion(strings.ToLower(trimmed)) {
		return AutoPlanDecision{
			Intent:     AutoPlanIntentExplanation,
			Action:     AutoPlanActionChat,
			Confidence: 0.95,
			Reason:     "request asks for an explanation rather than an implementation",
		}, nil
	}
	if shouldAutoPlanRequest(current, trimmed) {
		return AutoPlanDecision{
			Intent:     AutoPlanIntentImplementation,
			Action:     AutoPlanActionPlanExecute,
			Confidence: 0.9,
			Reason:     "implementation request has sufficient code or project context",
		}, nil
	}
	return AutoPlanDecision{
		Intent:     AutoPlanIntentConversation,
		Action:     AutoPlanActionChat,
		Confidence: 0.7,
		Reason:     "request is not a sufficiently grounded implementation task",
	}, nil
}
