package port

import (
	domainmessage "myai/core/domain/message"
	"time"

	agentplan "myai/core/plan"
	modelport "myai/core/port/model"
)

type ResponseMemoryStore interface {
	AppendAssistantMessageTo(sessionID string, message domainmessage.Message) error
	AddUsageTo(sessionID string, usage modelport.TokenUsage) error
	SetCurrentPlanForSession(sessionID string, currentPlan *agentplan.Plan) error
}

type PlanCapturer interface {
	Capture(sessionID string, goal string, content string, now time.Time) *agentplan.Plan
}
