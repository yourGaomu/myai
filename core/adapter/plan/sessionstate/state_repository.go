package sessionstate

import (
	"context"

	memorysession "myai/core/adapter/session/memory"
	agentplan "myai/core/plan"
)

type SaveSessionFunc func(ctx context.Context, sessionID string, model string) error

type Repository struct {
	sessions    *memorysession.Store
	saveSession SaveSessionFunc
}

func NewRepository(sessions *memorysession.Store, saveSession SaveSessionFunc) *Repository {
	return &Repository{
		sessions:    sessions,
		saveSession: saveSession,
	}
}

func (r *Repository) SaveCurrentPlan(ctx context.Context, sessionID string, model string, currentPlan *agentplan.Plan) error {
	if r == nil {
		return nil
	}
	currentPlan = agentplan.Clone(currentPlan)
	if r.sessions != nil {
		if err := r.sessions.SetCurrentPlanForSession(sessionID, currentPlan); err != nil {
			return err
		}
	}
	if r.saveSession != nil {
		return r.saveSession(ctx, sessionID, model)
	}
	return nil
}
