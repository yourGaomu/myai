package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	agentrunmemory "myai/core/adapter/persistence/memory/agentrun"
	agentruncommand "myai/core/application/agentrun/command"
	agentrunquery "myai/core/application/agentrun/query"
	agentrunservice "myai/core/application/agentrun/service"
	domainagentrun "myai/core/domain/agentrun"
	agentrunport "myai/core/port/agentrun"
)

func TestCommandAndQueryServicesPreserveRunTimeline(t *testing.T) {
	repository := agentrunmemory.New()
	clock := time.Date(2026, 7, 31, 1, 2, 3, 0, time.UTC)
	commands := agentrunservice.CommandService{Repository: repository, IDs: &sequenceIDs{}, Now: func() time.Time { return clock }}
	queries := agentrunservice.QueryService{Repository: repository}

	run, err := commands.Start(context.Background(), agentruncommand.Start{
		RequestID: "request-1", SessionID: "session-1", Kind: domainagentrun.KindPlan,
		Title: "Execute plan", TotalSteps: 2,
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	first, err := commands.Append(context.Background(), agentruncommand.Append{
		RunID: run.ID, Type: domainagentrun.EventTypeReasoning, Content: "inspect",
	})
	if err != nil {
		t.Fatalf("append first event: %v", err)
	}
	second, err := commands.Append(context.Background(), agentruncommand.Append{
		RunID: run.ID, Type: domainagentrun.EventTypePlanUpdate, CurrentStep: 1, TotalSteps: 2,
	})
	if err != nil {
		t.Fatalf("append second event: %v", err)
	}
	finished, err := commands.Finish(context.Background(), agentruncommand.Finish{
		RunID: run.ID, Status: domainagentrun.StatusSucceeded, CurrentStep: 2, TotalSteps: 2,
	})
	if err != nil {
		t.Fatalf("finish run: %v", err)
	}
	if first.Sequence != 1 || second.Sequence != 2 || finished.LastSequence != 3 {
		t.Fatalf("unexpected sequence state: first=%d second=%d last=%d", first.Sequence, second.Sequence, finished.LastSequence)
	}

	snapshots, err := queries.ListSessionRuns(context.Background(), agentrunquery.ListSessionRuns{SessionID: "session-1", Limit: 10})
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(snapshots) != 1 || len(snapshots[0].Events) != 3 {
		t.Fatalf("unexpected snapshots: %#v", snapshots)
	}
	if snapshots[0].Run.Status != domainagentrun.StatusSucceeded || snapshots[0].Events[2].Type != domainagentrun.EventTypeCompleted {
		t.Fatalf("unexpected terminal state: %#v", snapshots[0])
	}
}

func TestCommandServiceBoundsPersistenceWait(t *testing.T) {
	repository := blockingRunRepository{Repository: agentrunmemory.New()}
	commands := agentrunservice.CommandService{
		Repository:         repository,
		IDs:                &sequenceIDs{},
		PersistenceTimeout: 20 * time.Millisecond,
	}
	startedAt := time.Now()
	_, err := commands.Append(context.Background(), agentruncommand.Append{RunID: "run-1"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected persistence deadline, got %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("persistence timeout took too long: %s", elapsed)
	}
}

func TestCommandServiceRecoversInterruptedRuns(t *testing.T) {
	repository := agentrunmemory.New()
	commands := agentrunservice.CommandService{Repository: repository, IDs: &sequenceIDs{}}
	run, err := commands.Start(context.Background(), agentruncommand.Start{
		SessionID: "session-1", Kind: domainagentrun.KindPlan, Title: "Execute plan", TotalSteps: 3,
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if err := commands.RecoverRunning(context.Background()); err != nil {
		t.Fatalf("recover runs: %v", err)
	}
	recovered, err := repository.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get recovered run: %v", err)
	}
	if recovered.Status != domainagentrun.StatusFailed || recovered.FinishedAt == nil {
		t.Fatalf("unexpected recovered run: %#v", recovered)
	}
	if recovered.ErrorMessage != "Agent restarted before run completed" {
		t.Fatalf("unexpected recovery message: %q", recovered.ErrorMessage)
	}
	events, err := repository.ListEvents(context.Background(), []string{run.ID})
	if err != nil {
		t.Fatalf("list recovery events: %v", err)
	}
	if len(events) != 1 || events[0].Type != domainagentrun.EventTypeFailed {
		t.Fatalf("unexpected recovery events: %#v", events)
	}
}

type sequenceIDs struct{ next int }

func (ids *sequenceIDs) NewID() string {
	ids.next++
	return fmt.Sprintf("id-%d", ids.next)
}

type blockingRunRepository struct {
	agentrunport.Repository
}

func (blockingRunRepository) GetRun(ctx context.Context, _ string) (domainagentrun.Run, error) {
	<-ctx.Done()
	return domainagentrun.Run{}, ctx.Err()
}
