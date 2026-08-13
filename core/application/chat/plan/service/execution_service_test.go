package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	agentrunmemory "myai/core/adapter/persistence/memory/agentrun"
	agentrunservice "myai/core/application/agentrun/service"
	generationcommand "myai/core/application/chat/generation/command"
	generationresult "myai/core/application/chat/generation/result"
	plancommand "myai/core/application/chat/plan/command"
	messagecommand "myai/core/application/session/message/command"
	messageresult "myai/core/application/session/message/result"
	domainagentrun "myai/core/domain/agentrun"
	agentplan "myai/core/plan"
	agentrunport "myai/core/port/agentrun"
	modelport "myai/core/port/model"
	"myai/core/session"
)

func TestExecutionContinuesWhenAgentRunAppendTimesOut(t *testing.T) {
	current := &session.Session{
		ID: "session-1", Model: "model-1",
		CurrentPlan: &agentplan.Plan{
			ID: "plan-1", SessionID: "session-1", Goal: "finish the task", Status: agentplan.StatusApproved,
			Steps: []agentplan.Step{{ID: "step-1", Order: 1, Title: "implement", Status: agentplan.StepStatusPending}},
		},
	}
	repository := blockingGetRunRepository{Repository: agentrunmemory.New()}
	runs := agentrunservice.CommandService{
		Repository: repository, IDs: &planRunIDs{}, PersistenceTimeout: 20 * time.Millisecond,
	}
	generation := &recordingGenerationService{}
	runErrors := 0
	execution := ExecutionService{
		Models: fakeModelProvider{}, Sessions: fakeSessionLoader{current: current},
		Messages: fakeMessageAppender{current: current}, Generation: generation,
		Runs: runs, OnRunError: func(error) { runErrors++ },
	}

	startedAt := time.Now()
	result, err := execution.Execute(context.Background(), plancommand.Execute{SessionID: current.ID}, nil)
	if err != nil {
		t.Fatalf("execute plan: %v", err)
	}
	if generation.calls != 1 {
		t.Fatalf("expected first plan step generation, got %d calls", generation.calls)
	}
	if result.Plan == nil || result.Plan.Status != agentplan.StatusDone {
		t.Fatalf("unexpected completed plan: %#v", result.Plan)
	}
	if runErrors == 0 {
		t.Fatal("expected AgentRun persistence errors to be reported")
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("AgentRun persistence delayed the plan too long: %s", elapsed)
	}
}

type fakeModelProvider struct{}

func (fakeModelProvider) GetModel(string) modelport.ChatModelPort { return nil }

type fakeSessionLoader struct{ current *session.Session }

func (loader fakeSessionLoader) Load(context.Context, string) (*session.Session, error) {
	return loader.current, nil
}

type fakeMessageAppender struct{ current *session.Session }

func (appender fakeMessageAppender) AppendUserMessage(_ context.Context, command messagecommand.AppendUserMessage) (messageresult.Command, error) {
	return messageresult.Command{Session: appender.current, Input: command.Input, Appended: true}, nil
}

type recordingGenerationService struct{ calls int }

func (service *recordingGenerationService) Generate(_ context.Context, command generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
	service.calls++
	return generationresult.GenerationResponse{
		SessionID: command.Session.ID,
		Result:    modelport.ChatResult{Content: "step complete"},
	}, nil
}

type blockingGetRunRepository struct{ agentrunport.Repository }

func (blockingGetRunRepository) GetRun(ctx context.Context, _ string) (domainagentrun.Run, error) {
	<-ctx.Done()
	return domainagentrun.Run{}, ctx.Err()
}

type planRunIDs struct{ next int }

func (ids *planRunIDs) NewID() string {
	ids.next++
	return fmt.Sprintf("run-id-%d", ids.next)
}
