package service

import (
	"time"

	agentplan "myai/core/plan"
)

type CaptureService struct{}

func (CaptureService) Capture(sessionID string, goal string, content string, now time.Time) *agentplan.Plan {
	currentPlan := agentplan.NewDraft(sessionID, goal, content, now)
	if len(currentPlan.Steps) == 0 && agentplan.HasResultSection(content) {
		currentPlan.Status = agentplan.StatusDone
	}
	return currentPlan
}
