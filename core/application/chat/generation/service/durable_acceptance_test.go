package service

import (
	"context"
	"errors"
	"testing"

	memoryadapter "myai/core/adapter/session/memory"
	generationcommand "myai/core/application/chat/generation/command"
	generationresult "myai/core/application/chat/generation/result"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
	"myai/core/session"
)

type acceptanceModels struct{ model modelport.ChatModelPort }

func (m acceptanceModels) GetModel(string) modelport.ChatModelPort { return m.model }

type rejectingAssistantAcceptance struct {
	memory *memoryadapter.Store
	err    error
	calls  int
	legacy int
	t      *testing.T
}

func (p *rejectingAssistantAcceptance) PersistAssistant(*session.Session, modelport.ChatResult) {
	p.legacy++
}

func (p *rejectingAssistantAcceptance) AcceptAssistant(_ context.Context, current *session.Session, result modelport.ChatResult) error {
	p.calls++
	stored, err := p.memory.Snapshot(current.ID)
	if err != nil || len(stored.Messages) == 0 || stored.Messages[len(stored.Messages)-1].Text() != result.Content {
		p.t.Fatalf("assistant was not committed before acceptance: session=%+v error=%v", stored, err)
	}
	return p.err
}

func TestDurableAssistantRejectionReturnsCompletedAnswerAndError(t *testing.T) {
	memory := memoryadapter.NewStore("model")
	if err := memory.PutSession("session", "model", []domainmessage.Message{domainmessage.Text(domainmessage.RoleUser, "hello")}); err != nil {
		t.Fatal(err)
	}
	current, _ := memory.Snapshot("session")
	model := &pendingModel{responses: []modelport.ChatResult{{Content: "completed answer", Reasoning: "reason"}}}
	wantErr := errors.New("journal unavailable")
	persistence := &rejectingAssistantAcceptance{memory: memory, err: wantErr, t: t}
	response, err := (AssistantGenerationService{
		Models: acceptanceModels{model: model}, Contexts: pendingContextProvider{},
		AgentRunner:       AgentLoopService{Sessions: memory, Contexts: pendingContextProvider{}},
		ResponseCommitter: ResponseCommitService{Memory: memory}, Persistence: persistence,
	}).Generate(context.Background(), generationcommand.AssistantGeneration{Session: current})
	if !errors.Is(err, wantErr) || response.SessionID != "session" || response.Result.Content != "completed answer" {
		t.Fatalf("completed answer or persistence error lost: response=%+v error=%v", response, err)
	}
	if model.calls != 1 || persistence.calls != 1 || persistence.legacy != 0 {
		t.Fatalf("unexpected retries or void fallback: model=%d accept=%d legacy=%d", model.calls, persistence.calls, persistence.legacy)
	}
}

type rejectingToolAcceptance struct {
	memory *memoryadapter.Store
	err    error
	calls  int
	legacy int
	t      *testing.T
}

func (p *rejectingToolAcceptance) RecordToolExecution(context.Context, generationcommand.ToolExecutionRecord) {
	p.legacy++
}

func (p *rejectingToolAcceptance) AcceptToolExecution(_ context.Context, command generationcommand.ToolExecutionRecord) error {
	p.calls++
	stored, err := p.memory.Snapshot(command.SessionID)
	if err != nil || len(command.Messages) != 2 || len(stored.Messages) != 3 || !stored.Messages[1].HasToolCall() || stored.Messages[2].Role != domainmessage.RoleTool || len(stored.Messages[2].Parts) != 1 || stored.Messages[2].Parts[0].ToolResult == nil || stored.Messages[2].Parts[0].ToolResult.Content != "done" {
		p.t.Fatalf("executed tool evidence missing before acceptance: stored=%+v record=%+v error=%v", stored, command, err)
	}
	return p.err
}

func TestDurableToolRejectionRetainsEvidenceAndStopsWithoutReplay(t *testing.T) {
	memory := memoryadapter.NewStore("model")
	if err := memory.PutSession("session", "model", []domainmessage.Message{domainmessage.Text(domainmessage.RoleUser, "inspect")}); err != nil {
		t.Fatal(err)
	}
	current, _ := memory.Snapshot("session")
	model := &pendingModel{responses: []modelport.ChatResult{
		{ToolCalls: []domainmessage.ToolCall{{ID: "read-1", Name: "read_file", Arguments: `{}`}}},
		{Content: "must not sample again"},
	}}
	executor := &recoveryExecutor{}
	wantErr := errors.New("tool journal rejected")
	records := &rejectingToolAcceptance{memory: memory, err: wantErr, t: t}
	_, err := (AgentLoopService{Sessions: memory, Contexts: pendingContextProvider{}, ToolExecutor: executor, ToolRecords: records}).Run(context.Background(), generationcommand.Run{Session: current, Model: model})
	if !errors.Is(err, wantErr) {
		t.Fatalf("tool persistence rejection not returned: %v", err)
	}
	if model.calls != 1 || executor.calls != 1 || records.calls != 1 || records.legacy != 0 {
		t.Fatalf("tool replayed or model continued: model=%d tool=%d accept=%d legacy=%d", model.calls, executor.calls, records.calls, records.legacy)
	}
}

type acceptanceTurnHooks struct{}

func (acceptanceTurnHooks) Handle(context.Context, generationcommand.TurnHook) (generationcommand.TurnHookOutcome, error) {
	return generationcommand.TurnHookOutcome{Continuation: "persist this context first"}, nil
}

type acceptanceGenerator struct{ calls int }

func (g *acceptanceGenerator) Generate(context.Context, generationcommand.AssistantGeneration) (generationresult.GenerationResponse, error) {
	g.calls++
	return generationresult.GenerationResponse{}, nil
}

func TestDurableTurnHookRejectionStopsBeforeGeneration(t *testing.T) {
	generator := &acceptanceGenerator{}
	wantErr := errors.New("context journal rejected")
	accepted, legacy := 0, 0
	_, err := (TaskService{
		RequestIDs: taskRequestIDStub{}, Generator: generator, TurnHooks: acceptanceTurnHooks{},
		OnMessagesAccepted: func(_ context.Context, current *session.Session, messages []domainmessage.Message) error {
			accepted++
			if len(messages) != 1 || len(current.Messages) != 2 || !messages[0].IsSynthetic() {
				t.Fatalf("hook context missing before acceptance: current=%+v messages=%+v", current, messages)
			}
			return wantErr
		},
		OnMessagesAppended: func(*session.Session, []domainmessage.Message) { legacy++ },
	}).Generate(context.Background(), generationcommand.GenerationTask{Session: &session.Session{ID: "session", Messages: []domainmessage.Message{domainmessage.Text(domainmessage.RoleUser, "hello")}}})
	if !errors.Is(err, wantErr) || generator.calls != 0 || accepted != 1 || legacy != 0 {
		t.Fatalf("hook rejection did not stop generation: error=%v generator=%d accept=%d legacy=%d", err, generator.calls, accepted, legacy)
	}
}
