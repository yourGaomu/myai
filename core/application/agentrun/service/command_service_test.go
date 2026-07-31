package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	agentrunmemory "myai/core/adapter/persistence/memory/agentrun"
	agentruncommand "myai/core/application/agentrun/command"
	agentrunquery "myai/core/application/agentrun/query"
	agentrunservice "myai/core/application/agentrun/service"
	domainagentrun "myai/core/domain/agentrun"
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

type sequenceIDs struct{ next int }

func (ids *sequenceIDs) NewID() string {
	ids.next++
	return fmt.Sprintf("id-%d", ids.next)
}
