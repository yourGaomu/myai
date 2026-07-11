package service

import (
	"time"

	agentplan "myai/core/plan"
)

type CaptureService struct{}

func (CaptureService) Capture(sessionID string, goal string, content string, now time.Time) *agentplan.Plan {
	currentPlan := agentplan.NewDraft(sessionID, goal, content, now)
	if agentplan.HasResultSection(content) {
		currentPlan.Status = agentplan.StatusDone
		for index := range currentPlan.Steps {
			currentPlan.Steps[index].Status = agentplan.StepStatusDone
		}
	}
	return currentPlan
}
