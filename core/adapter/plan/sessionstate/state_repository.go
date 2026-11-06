package sessionstate

import (
	"context"
	"sync"

	memorysession "myai/core/adapter/session/memory"
	agentplan "myai/core/plan"
	repository "myai/core/port/repository"
)

type SaveSessionFunc func(ctx context.Context, sessionID string, model string) error

type Repository struct {
	sessions    *memorysession.Store
	saveSession SaveSessionFunc
	// Production uses narrow CAS persistence; the callback remains for older adapters.
	Plans         repository.SessionPlanStore
	EnsureSession SaveSessionFunc
	mu            sync.Mutex
}

func NewRepository(sessions *memorysession.Store, saveSession SaveSessionFunc) *Repository {
	return &Repository{sessions: sessions, saveSession: saveSession}
}

func (r *Repository) SaveCurrentPlan(ctx context.Context, sessionID, model string, next *agentplan.Plan) error {
	return r.save(ctx, sessionID, model, next, false, "", 0)
}

func (r *Repository) SaveCurrentPlanIfUnchanged(ctx context.Context, sessionID, model string, next *agentplan.Plan, expectedID string, expectedRevision int64) error {
	return r.save(ctx, sessionID, model, next, true, expectedID, expectedRevision)
}

func (r *Repository) save(ctx context.Context, sessionID, model string, next *agentplan.Plan, check bool, expectedID string, expectedRevision int64) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions == nil {
		return nil
	}
	current, err := r.sessions.Snapshot(sessionID)
	if err != nil {
		return err
	}
	actualID, actualRevision := "", int64(0)
	if current.CurrentPlan != nil {
		actualID, actualRevision = current.CurrentPlan.ID, current.CurrentPlan.Revision
	}
	if check {
		if actualID != expectedID || actualRevision != expectedRevision {
			return repository.ErrConflict
		}
	} else {
		expectedID, expectedRevision = actualID, actualRevision
		if next != nil && actualID != "" && (next.ID != actualID || next.Revision != actualRevision+1) {
			return repository.ErrConflict
		}
	}
	if r.Plans != nil {
		if r.EnsureSession != nil {
			if err = r.EnsureSession(ctx, sessionID, model); err != nil {
				return err
			}
		}
		if err = r.Plans.SaveSessionPlan(ctx, sessionID, expectedID, expectedRevision, next); err != nil {
			return err
		}
		return r.sessions.CompareAndSwapPlan(sessionID, expectedID, expectedRevision, next)
	}
	if err = r.sessions.CompareAndSwapPlan(sessionID, expectedID, expectedRevision, next); err != nil {
		return err
	}
	if r.saveSession != nil {
		if err = r.saveSession(ctx, sessionID, model); err != nil {
			id, revision := "", int64(0)
			if next != nil {
				id, revision = next.ID, next.Revision
			}
			_ = r.sessions.CompareAndSwapPlan(sessionID, id, revision, current.CurrentPlan)
			return err
		}
	}
	return nil
}
