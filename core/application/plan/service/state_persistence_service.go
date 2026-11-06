package service

import (
	"context"
	"time"

	plancommand "myai/core/application/plan/command"
	agentplan "myai/core/plan"
	planport "myai/core/port/plan"
)

type StatePersistenceService struct {
	Repository planport.StateRepository
	Now        func() time.Time
}

func (s StatePersistenceService) Save(ctx context.Context, command plancommand.SaveState) (*agentplan.Plan, error) {
	currentPlan := agentplan.Clone(command.Plan)
	if currentPlan != nil {
		now := time.Now()
		if s.Now != nil {
			now = s.Now()
		}
		currentPlan.UpdatedAt = now
		currentPlan.Revision++
	}
	if s.Repository != nil {
		if conditional, ok := s.Repository.(interface {
			SaveCurrentPlanIfUnchanged(context.Context, string, string, *agentplan.Plan, string, int64) error
		}); ok && command.CheckExpected {
			if err := conditional.SaveCurrentPlanIfUnchanged(ctx, command.SessionID, command.Model, currentPlan, command.ExpectedPlanID, command.ExpectedRevision); err != nil {
				return nil, err
			}
			return currentPlan, nil
		}
		if err := s.Repository.SaveCurrentPlan(ctx, command.SessionID, command.Model, currentPlan); err != nil {
			return nil, err
		}
	}
	return currentPlan, nil
}
