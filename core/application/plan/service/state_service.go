package service

import agentplan "myai/core/plan"

type StateService struct{}

func (StateService) Start(currentPlan *agentplan.Plan) *agentplan.Plan {
	currentPlan = agentplan.Clone(currentPlan)
	if currentPlan == nil {
		return nil
	}
	currentPlan.Status = agentplan.StatusRunning
	for index := range currentPlan.Steps {
		currentPlan.Steps[index].Status = agentplan.StepStatusPending
	}
	return currentPlan
}

func (StateService) MarkStepRunning(currentPlan *agentplan.Plan, index int) *agentplan.Plan {
	currentPlan = agentplan.Clone(currentPlan)
	if !hasStep(currentPlan, index) {
		return currentPlan
	}
	currentPlan.Steps[index].Status = agentplan.StepStatusRunning
	return currentPlan
}

func (StateService) MarkStepDone(currentPlan *agentplan.Plan, index int) *agentplan.Plan {
	currentPlan = agentplan.Clone(currentPlan)
	if !hasStep(currentPlan, index) {
		return currentPlan
	}
	currentPlan.Steps[index].Status = agentplan.StepStatusDone
	return currentPlan
}

func (StateService) MarkStepFailed(currentPlan *agentplan.Plan, index int) *agentplan.Plan {
	currentPlan = agentplan.Clone(currentPlan)
	if currentPlan == nil {
		return nil
	}
	currentPlan.Status = agentplan.StatusFailed
	if hasStep(currentPlan, index) {
		currentPlan.Steps[index].Status = agentplan.StepStatusFailed
	}
	return currentPlan
}

func (StateService) MarkCanceled(currentPlan *agentplan.Plan) *agentplan.Plan {
	currentPlan = agentplan.Clone(currentPlan)
	if currentPlan == nil {
		return nil
	}
	currentPlan.Status = agentplan.StatusCanceled
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
