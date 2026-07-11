package chat

import (
	"context"
	"errors"
	"testing"

	plancommand "myai/core/application/plan/command"
	messagecommand "myai/core/application/session/message/command"
	messageresult "myai/core/application/session/message/result"
	"myai/core/contextmgr"
	agentplan "myai/core/plan"
	modelport "myai/core/port/model"
	"myai/core/session"
)

func TestPlanExecutionServiceExecutesStepsAndCombinesResults(t *testing.T) {
	current := planExecutionSession()
	states := &recordingPlanStateStore{}
	generation := &recordingPlanGeneration{responses: []GenerationResponse{
		{SessionID: current.ID, Result: modelport.ChatResult{Content: "first", Usage: modelport.TokenUsage{TotalTokens: 2}}, Context: contextmgr.Info{WindowK: 10}},
		{SessionID: current.ID, Result: modelport.ChatResult{Content: "second", Usage: modelport.TokenUsage{TotalTokens: 3}}, Context: contextmgr.Info{WindowK: 20}},
	}}
	userMessages := &recordingPlanUserMessages{}
	events := &recordingPlanEvents{}
	updates := &recordingPlanUpdates{}

	result, err := (PlanExecutionService{
		Models:       &assistantModelProvider{},
		Sessions:     staticPlanSessionLoader{current: current},
		Messages:     &planMessageAppender{current: current},
		Generation:   generation,
		PlanStates:   states,
		UserMessages: userMessages,
		Events:       events,
	}).Execute(context.Background(), PlanExecutionCommand{SessionID: current.ID}, updates)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Result.Content != "first\n\nsecond" || result.Result.Usage.TotalTokens != 5 {
		t.Fatalf("unexpected combined result: %#v", result.Result)
	}
	if result.Context.WindowK != 20 || result.Plan == nil || result.Plan.Status != agentplan.StatusDone {
		t.Fatalf("unexpected execution result: %#v", result)
	}
	if len(generation.commands) != 2 || !generation.commands[0].ForceChatMode || generation.commands[0].CapturePlan {
		t.Fatalf("unexpected generation commands: %#v", generation.commands)
	}
	if generation.commands[0].Title != "Execute plan" || generation.commands[1].Title != "" {
		t.Fatalf("unexpected step titles: %#v", generation.commands)
	}
	if len(userMessages.commands) != 2 || len(updates.plans) != 6 || events.count != 6 {
		t.Fatalf("unexpected side effects: user=%d updates=%d events=%d", len(userMessages.commands), len(updates.plans), events.count)
	}
	if len(states.plans) != 6 || states.plans[len(states.plans)-1].Status != agentplan.StatusDone {
		t.Fatalf("unexpected persisted states: %#v", states.plans)
	}
}

func TestPlanExecutionServiceMarksStepFailedWhenGenerationFails(t *testing.T) {
	current := planExecutionSession()
	states := &recordingPlanStateStore{}
	generationErr := errors.New("generation failed")

	_, err := (PlanExecutionService{
		Models:     &assistantModelProvider{},
		Sessions:   staticPlanSessionLoader{current: current},
		Messages:   &planMessageAppender{current: current},
		Generation: &recordingPlanGeneration{err: generationErr},
		PlanStates: states,
	}).Execute(context.Background(), PlanExecutionCommand{SessionID: current.ID}, nil)
	if !errors.Is(err, generationErr) {
		t.Fatalf("Execute() error = %v, want generation failure", err)
	}
	last := states.plans[len(states.plans)-1]
	if last.Status != agentplan.StatusFailed || last.Steps[0].Status != agentplan.StepStatusFailed {
		t.Fatalf("unexpected failed state: %#v", last)
	}
}

func planExecutionSession() *session.Session {
	return &session.Session{
		ID:    "session-1",
		Model: "model-1",
		CurrentPlan: &agentplan.Plan{
			ID:        "plan-1",
			SessionID: "session-1",
			Goal:      "finish work",
			Status:    agentplan.StatusApproved,
			Steps: []agentplan.Step{
				{ID: "step-1", Order: 1, Title: "First", Status: agentplan.StepStatusPending},
				{ID: "step-2", Order: 2, Title: "Second", Status: agentplan.StepStatusPending},
			},
		},
	}
}

type staticPlanSessionLoader struct {
	current *session.Session
}

func (l staticPlanSessionLoader) Load(context.Context, string) (*session.Session, error) {
	return l.current, nil
}

type planMessageAppender struct {
	current *session.Session
}

func (a *planMessageAppender) AppendUserMessage(_ context.Context, command messagecommand.AppendUserMessage) (messageresult.Command, error) {
	a.current.AddUserMessage(command.Input)
	return messageresult.Command{Session: a.current, Input: command.Input}, nil
}

type recordingPlanGeneration struct {
	commands  []GenerationTaskCommand
	responses []GenerationResponse
	err       error
}

func (g *recordingPlanGeneration) Generate(_ context.Context, command GenerationTaskCommand) (GenerationResponse, error) {
	g.commands = append(g.commands, command)
	if g.err != nil {
		return GenerationResponse{}, g.err
	}
	response := g.responses[len(g.commands)-1]
	return response, nil
}

type recordingPlanStateStore struct {
	plans []*agentplan.Plan
}

func (s *recordingPlanStateStore) Save(_ context.Context, command plancommand.SaveState) (*agentplan.Plan, error) {
	current := agentplan.Clone(command.Plan)
	s.plans = append(s.plans, current)
	return current, nil
}

type recordingPlanUserMessages struct {
	commands []PersistUserMessageCommand
}

func (p *recordingPlanUserMessages) PersistUserMessage(command PersistUserMessageCommand) {
	p.commands = append(p.commands, command)
}

type recordingPlanEvents struct {
	count int
}

func (e *recordingPlanEvents) SessionChanged(context.Context, string, string) {
	e.count++
}

type recordingPlanUpdates struct {
	plans []*agentplan.Plan
}

func (u *recordingPlanUpdates) PlanUpdated(currentPlan *agentplan.Plan) {
	u.plans = append(u.plans, agentplan.Clone(currentPlan))
}
