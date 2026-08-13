package service

import (
	"context"
	"testing"

	agentrunmemory "myai/core/adapter/persistence/memory/agentrun"
	agentrunservice "myai/core/application/agentrun/service"
	generationcommand "myai/core/application/chat/generation/command"
	generationresult "myai/core/application/chat/generation/result"
	domainagentrun "myai/core/domain/agentrun"
	agentplan "myai/core/plan"
	modelport "myai/core/port/model"
	"myai/core/session"
)

func TestTaskServicePublishesCapturedPlan(t *testing.T) {
	repository := agentrunmemory.New()
	runs := agentrunservice.CommandService{Repository: repository, IDs: &runStreamIDs{}}
	currentPlan := &agentplan.Plan{
		ID: "plan-1", SessionID: "session-1", Goal: "inspect startup", Status: agentplan.StatusDraft,
		Steps: []agentplan.Step{
			{ID: "step-1", Order: 1, Title: "Inspect main", Status: agentplan.StepStatusPending},
			{ID: "step-2", Order: 2, Title: "Inspect commands", Status: agentplan.StepStatusPending},
		},
	}
	var started domainagentrun.Run
	var liveEvents []domainagentrun.Event

	_, err := (TaskService{
		RequestIDs: taskRequestIDStub{},
		Generator:  taskGeneratorStub{response: generationresult.GenerationResponse{SessionID: "session-1", Plan: currentPlan}},
		Runs:       runs,
	}).Generate(context.Background(), generationcommand.GenerationTask{
		Session: &session.Session{ID: "session-1", Model: "model-1", AgentMode: session.AgentModePlan},
		Reason:  "user request",
		Stream: modelport.ChatStreamHandler{
			CorrelationID: "remote-request-1",
			OnRunStarted:  func(run domainagentrun.Run) { started = run },
			OnRunEvent:    func(event domainagentrun.Event) { liveEvents = append(liveEvents, event) },
		},
		CapturePlan: true,
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if started.Kind != domainagentrun.KindPlan {
		t.Fatalf("expected plan run kind, got %q", started.Kind)
	}
	if len(liveEvents) != 1 || liveEvents[0].Type != domainagentrun.EventTypePlanUpdate {
		t.Fatalf("expected live plan update, got %#v", liveEvents)
	}
	if liveEvents[0].Status != agentplan.StatusDraft || liveEvents[0].TotalSteps != 2 {
		t.Fatalf("unexpected plan progress event: %#v", liveEvents[0])
	}

	persisted, err := repository.ListEvents(context.Background(), []string{started.ID})
	if err != nil {
		t.Fatalf("list run events: %v", err)
	}
	if len(persisted) != 2 || persisted[0].Type != domainagentrun.EventTypePlanUpdate || persisted[1].Type != domainagentrun.EventTypeCompleted {
		t.Fatalf("unexpected persisted events: %#v", persisted)
	}
}

type taskRequestIDStub struct{}

func (taskRequestIDStub) NewRequestID() string { return "generation-request-1" }

type taskGeneratorStub struct {
	response generationresult.GenerationResponse
}

func (s taskGeneratorStub) Generate(context.Context, generationcommand.AssistantGeneration) (generationresult.GenerationResponse, error) {
	return s.response, nil
}
