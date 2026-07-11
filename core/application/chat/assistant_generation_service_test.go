package chat

import (
	"context"
	"errors"
	"testing"

	"myai/core/contextmgr"
	domainmessage "myai/core/domain/message"
	agentplan "myai/core/plan"
	modelport "myai/core/port/model"
	"myai/core/session"
)

func TestAssistantGenerationServiceGenerateOrchestratesUseCase(t *testing.T) {
	current := assistantGenerationSession()
	model := &assistantGenerationModel{}
	models := &assistantModelProvider{models: map[string]modelport.ChatModelPort{"model-a": model}}
	runtime := &assistantRuntimeProvider{prompt: "runtime prompt"}
	contexts := &assistantContextProvider{info: contextmgr.Info{WindowK: 32, PrefixHash: "prefix"}}
	compactor := &assistantCompactor{info: CompactInfo{Triggered: true, BeforeTokens: 20, AfterTokens: 10}}
	runner := &assistantRunner{result: modelport.ChatResult{Content: "answer", Usage: modelport.TokenUsage{TotalTokens: 3, Available: true}}}
	plan := &agentplan.Plan{ID: "plan-1", SessionID: "session-1", Status: agentplan.StatusDraft}
	committer := &assistantCommitter{result: CommitResult{Plan: plan}}
	persistence := &assistantPersistence{}

	response, err := AssistantGenerationService{
		Models:              models,
		RuntimeInstructions: runtime,
		Contexts:            contexts,
		Compactor:           compactor,
		AgentRunner:         runner,
		ResponseCommitter:   committer,
		Persistence:         persistence,
	}.Generate(context.Background(), AssistantGenerationCommand{
		Session:       current,
		LatestInput:   "write a poem",
		RequestID:     "request-1",
		CapturePlan:   true,
		ForceChatMode: true,
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if response.SessionID != "session-1" || response.Result.Content != "answer" {
		t.Fatalf("unexpected response: %#v", response)
	}
	if response.Compact.BeforeTokens != 20 || !response.Compact.Triggered {
		t.Fatalf("expected compact info to be returned, got %#v", response.Compact)
	}
	if response.Context.WindowK != 32 || response.Context.PrefixHash != "prefix" {
		t.Fatalf("expected context info from provider, got %#v", response.Context)
	}
	if response.Plan == nil || response.Plan.ID != "plan-1" {
		t.Fatalf("expected commit plan, got %#v", response.Plan)
	}
	if models.requestedName != "model-a" {
		t.Fatalf("expected model lookup by session model, got %q", models.requestedName)
	}
	if runtime.input != "write a poem" || !runtime.forceChatMode {
		t.Fatalf("expected runtime input and force flag, got %#v", runtime)
	}
	if compactor.runtimePrompt != "runtime prompt" || compactor.model != model {
		t.Fatalf("expected compactor to receive runtime prompt and model, got %#v", compactor)
	}
	if runner.command.RequestID != "request-1" || runner.command.RuntimePrompt != "runtime prompt" || !runner.command.ForceChatMode {
		t.Fatalf("expected runner command to be forwarded, got %#v", runner.command)
	}
	if !committer.command.CapturePlan || committer.command.Result.Content != "answer" {
		t.Fatalf("expected commit command to include result and plan flag, got %#v", committer.command)
	}
	if !persistence.assistantPersisted || persistence.currentSessionID != "session-1" {
		t.Fatalf("expected persistence sink to be called, got %#v", persistence)
	}
}

func TestAssistantGenerationServiceCompactErrorIsNonFatal(t *testing.T) {
	expected := errors.New("compact failed")
	var compactErr error

	response, err := AssistantGenerationService{
		Models:              &assistantModelProvider{models: map[string]modelport.ChatModelPort{"model-a": &assistantGenerationModel{}}},
		RuntimeInstructions: &assistantRuntimeProvider{prompt: "runtime prompt"},
		Contexts:            &assistantContextProvider{},
		Compactor:           &assistantCompactor{err: expected},
		AgentRunner:         &assistantRunner{result: modelport.ChatResult{Content: "answer"}},
		ResponseCommitter:   &assistantCommitter{},
		OnCompactError: func(err error) {
			compactErr = err
		},
	}.Generate(context.Background(), AssistantGenerationCommand{
		Session: assistantGenerationSession(),
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if !errors.Is(compactErr, expected) {
		t.Fatalf("expected compact error callback, got %v", compactErr)
	}
	if response.Compact.Triggered {
		t.Fatalf("expected compact info to stay empty after compact failure, got %#v", response.Compact)
	}
	if response.Result.Content != "answer" {
		t.Fatalf("expected generation to continue after compact failure, got %#v", response.Result)
	}
}

func TestAssistantGenerationServiceReturnsModelNotFound(t *testing.T) {
	_, err := AssistantGenerationService{
		Models:              &assistantModelProvider{},
		RuntimeInstructions: &assistantRuntimeProvider{},
		AgentRunner:         &assistantRunner{},
		ResponseCommitter:   &assistantCommitter{},
	}.Generate(context.Background(), AssistantGenerationCommand{
		Session: assistantGenerationSession(),
	})
	if err == nil || err.Error() != "model not found: model-a" {
		t.Fatalf("expected model not found error, got %v", err)
	}
}

type assistantGenerationModel struct{}

func (m *assistantGenerationModel) Generate(ctx context.Context, request modelport.GenerateRequest) (modelport.ChatResult, error) {
	return modelport.ChatResult{}, errors.New("assistantGenerationModel should not be called directly")
}

type assistantModelProvider struct {
	models        map[string]modelport.ChatModelPort
	requestedName string
}

func (p *assistantModelProvider) GetModel(name string) modelport.ChatModelPort {
	p.requestedName = name
	if p.models == nil {
		return nil
	}
	return p.models[name]
}

type assistantRuntimeProvider struct {
	prompt        string
	input         string
	forceChatMode bool
}

func (p *assistantRuntimeProvider) Prompt(ctx context.Context, current *session.Session, input string, forceChatMode bool) string {
	p.input = input
	p.forceChatMode = forceChatMode
	return p.prompt
}

type assistantContextProvider struct {
	runtimePrompt string
	info          contextmgr.Info
}

func (p *assistantContextProvider) Snapshot(current *session.Session, runtimePrompt string) contextmgr.Snapshot {
	p.runtimePrompt = runtimePrompt
	return contextmgr.Snapshot{
		Info: p.info,
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleSystem, "system"),
		},
	}
}

type assistantCompactor struct {
	model         modelport.ChatModelPort
	runtimePrompt string
	info          CompactInfo
	err           error
}

func (c *assistantCompactor) CompactIfNeeded(ctx context.Context, current *session.Session, model modelport.ChatModelPort, runtimePrompt string) (CompactInfo, error) {
	c.model = model
	c.runtimePrompt = runtimePrompt
	if c.err != nil {
		return CompactInfo{}, c.err
	}
	return c.info, nil
}

type assistantRunner struct {
	command RunCommand
	result  modelport.ChatResult
	err     error
}

func (r *assistantRunner) Run(ctx context.Context, command RunCommand) (modelport.ChatResult, error) {
	r.command = command
	if r.err != nil {
		return modelport.ChatResult{}, r.err
	}
	return r.result, nil
}

type assistantCommitter struct {
	command CommitCommand
	result  CommitResult
	err     error
}

func (c *assistantCommitter) Commit(command CommitCommand) (CommitResult, error) {
	c.command = command
	if c.err != nil {
		return CommitResult{}, c.err
	}
	return c.result, nil
}

type assistantPersistence struct {
	assistantPersisted bool
	result             modelport.ChatResult
	currentSessionID   string
}

func (p *assistantPersistence) PersistAssistant(current *session.Session, result modelport.ChatResult) {
	p.assistantPersisted = true
	p.result = result
}

func (p *assistantPersistence) PersistCurrentSession(sessionID string) {
	p.currentSessionID = sessionID
}

func assistantGenerationSession() *session.Session {
	return &session.Session{
		ID:    "session-1",
		Model: "model-a",
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleSystem, "system"),
		},
	}
}
