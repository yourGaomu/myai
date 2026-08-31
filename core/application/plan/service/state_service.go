package service

import (
	"strings"
	"time"

	agentplan "myai/core/plan"
)

type StateService struct{}

func (StateService) Approve(currentPlan *agentplan.Plan) *agentplan.Plan {
	currentPlan = agentplan.Clone(currentPlan)
	if currentPlan == nil {
		return nil
	}
	currentPlan.Status = agentplan.StatusApproved
	return currentPlan
}

func (StateService) Start(currentPlan *agentplan.Plan) *agentplan.Plan {
	currentPlan = agentplan.Clone(currentPlan)
	if currentPlan == nil {
		return nil
	}
	currentPlan.Status = agentplan.StatusRunning
	for index := range currentPlan.Steps {
		switch currentPlan.Steps[index].Status {
		case agentplan.StepStatusDone, agentplan.StepStatusSkipped:
			continue
		default:
			currentPlan.Steps[index].Status = agentplan.StepStatusPending
		}
	}
	return currentPlan
}

func (StateService) MarkStepRunning(currentPlan *agentplan.Plan, index int) *agentplan.Plan {
	currentPlan = agentplan.Clone(currentPlan)
	if !hasStep(currentPlan, index) {
		return currentPlan
	}
	currentPlan.Steps[index].Status = agentplan.StepStatusRunning
	now := time.Now()
	currentPlan.Steps[index].StartedAt = &now
	currentPlan.Steps[index].CompletedAt = nil
	return currentPlan
}

func (StateService) MarkStepDone(currentPlan *agentplan.Plan, index int) *agentplan.Plan {
	currentPlan = agentplan.Clone(currentPlan)
	if !hasStep(currentPlan, index) {
		return currentPlan
	}
	currentPlan.Steps[index].Status = agentplan.StepStatusDone
	now := time.Now()
	currentPlan.Steps[index].CompletedAt = &now
	currentPlan.Steps[index].LastError = ""
	return currentPlan
}

func (StateService) MarkStepFailed(currentPlan *agentplan.Plan, index int, cause ...string) *agentplan.Plan {
	currentPlan = agentplan.Clone(currentPlan)
	if currentPlan == nil {
		return nil
	}
	currentPlan.Status = agentplan.StatusFailed
	if hasStep(currentPlan, index) {
		currentPlan.Steps[index].Status = agentplan.StepStatusFailed
		if len(cause) > 0 {
			currentPlan.Steps[index].LastError = strings.TrimSpace(cause[0])
		}
		now := time.Now()
		currentPlan.Steps[index].CompletedAt = &now
	}
	return currentPlan
}

// MarkStepRetry records a failed attempt while keeping the plan resumable.
// The next execution pass can safely pick the step up from pending status.
func (StateService) MarkStepRetry(currentPlan *agentplan.Plan, index int, cause string) *agentplan.Plan {
	currentPlan = agentplan.Clone(currentPlan)
	if !hasStep(currentPlan, index) {
		return currentPlan
	}
	step := &currentPlan.Steps[index]
	step.RetryCount++
	step.Status = agentplan.StepStatusPending
	step.LastError = strings.TrimSpace(cause)
	step.CompletedAt = nil
	return currentPlan
}

func (StateService) MarkStepSkipped(currentPlan *agentplan.Plan, index int, cause string) *agentplan.Plan {
	currentPlan = agentplan.Clone(currentPlan)
	if !hasStep(currentPlan, index) {
		return currentPlan
	}
	step := &currentPlan.Steps[index]
	step.Status = agentplan.StepStatusSkipped
	step.LastError = strings.TrimSpace(cause)
	now := time.Now()
	step.CompletedAt = &now
	return currentPlan
}

func (StateService) MarkCanceled(currentPlan *agentplan.Plan) *agentplan.Plan {
	currentPlan = agentplan.Clone(currentPlan)
	if currentPlan == nil {
		return nil
	}
	currentPlan.Status = agentplan.StatusCanceled
	for index := range currentPlan.Steps {
		if currentPlan.Steps[index].Status == agentplan.StepStatusRunning {
			currentPlan.Steps[index].Status = agentplan.StepStatusPending
		}
	}
	return currentPlan
}

func (StateService) MarkDone(currentPlan *agentplan.Plan) *agentplan.Plan {
	currentPlan = agentplan.Clone(currentPlan)
	if currentPlan == nil {
		return nil
	}
	currentPlan.Status = agentplan.StatusDone
	return currentPlan
}

func hasStep(currentPlan *agentplan.Plan, index int) bool {
	return currentPlan != nil && index >= 0 && index < len(currentPlan.Steps)
}
