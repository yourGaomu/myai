package port

import (
	"time"

	agentplan "myai/core/plan"
	modelport "myai/core/port/model"
	sessionport "myai/core/port/session"
)

type ResponseMemoryStore interface {
	sessionport.TranscriptStore
	AddUsageTo(sessionID string, usage modelport.TokenUsage) error
	SetCurrentPlanForSession(sessionID string, currentPlan *agentplan.Plan) error
}

type PlanCapturer interface {
	Capture(sessionID string, goal string, content string, now time.Time) *agentplan.Plan
}
