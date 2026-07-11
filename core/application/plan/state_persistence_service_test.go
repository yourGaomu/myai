package plan

import (
	"context"
	"testing"
	"time"

	agentplan "myai/core/plan"
)

type fakeStateRepository struct {
	sessionID string
	model     string
	plan      *agentplan.Plan
}

func (r *fakeStateRepository) SaveCurrentPlan(_ context.Context, sessionID string, model string, currentPlan *agentplan.Plan) error {
	r.sessionID = sessionID
	r.model = model
	r.plan = agentplan.Clone(currentPlan)
	return nil
}

func TestStatePersistenceServiceClonesAndUpdatesTimestamp(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	repository := &fakeStateRepository{}
	currentPlan := &agentplan.Plan{
		Status: agentplan.StatusRunning,
		Steps: []agentplan.Step{
			{Status: agentplan.StepStatusPending},
		},
	}

	saved, err := (StatePersistenceService{
		Repository: repository,
		Now:        func() time.Time { return now },
	}).Save(context.Background(), SaveStateCommand{
		SessionID: "session-1",
		Model:     "gpt-test",
		Plan:      currentPlan,
	})
	if err != nil {
		t.Fatal(err)
	}

	if !saved.UpdatedAt.Equal(now) {
		t.Fatalf("expected updated timestamp %s, got %s", now, saved.UpdatedAt)
	}
	if repository.sessionID != "session-1" || repository.model != "gpt-test" {
		t.Fatalf("unexpected repository command: %#v", repository)
	}
	if repository.plan == currentPlan || saved == currentPlan {
		t.Fatal("expected plan to be cloned")
	}
	if currentPlan.UpdatedAt.Equal(now) {
		t.Fatal("expected original plan to remain unchanged")
	}
}
