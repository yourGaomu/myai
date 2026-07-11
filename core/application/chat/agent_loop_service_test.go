package chat

import (
	"context"
	"errors"
	"testing"

	"myai/core/contextmgr"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	modelport "myai/core/port/model"
	"myai/core/session"
)

func TestAgentLoopServiceReturnsWhenModelDoesNotRequestTools(t *testing.T) {
	current := testSession()
	model := &scriptedModel{
		results: []modelport.ChatResult{
			{
				Content:   "done",
				Reasoning: "thinking",
				Usage:     modelport.TokenUsage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5, Available: true},
			},
		},
	}
	contexts := &recordingContextProvider{}
	tools := &recordingToolCatalog{}
	executor := &recordingToolExecutor{}

	result, err := AgentLoopService{
		Contexts:     contexts,
		Tools:        tools,
		ToolExecutor: executor,
	}.Run(context.Background(), RunCommand{
		Model:         model,
		Session:       current,
		RuntimePrompt: "runtime",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if result.Content != "done" {
		t.Fatalf("expected final content, got %q", result.Content)
	}
	if result.Reasoning != "thinking" {
		t.Fatalf("expected reasoning to be preserved, got %q", result.Reasoning)
	}
	if result.Usage.TotalTokens != 5 || !result.Usage.Available {
		t.Fatalf("expected usage to be returned, got %#v", result.Usage)
	}
	if len(current.Messages) != 2 {
		t.Fatalf("expected no tool messages to be appended, got %d messages", len(current.Messages))
	}
	if executor.calls != 0 {
		t.Fatalf("expected tool executor not to run, got %d calls", executor.calls)
	}
	if len(model.requests) != 1 || len(model.requests[0].Tools) != 1 {
		t.Fatalf("expected one model request with available tools, got %#v", model.requests)
	}
	if got := contexts.prompts; len(got) != 1 || got[0] != "runtime" {
		t.Fatalf("expected initial runtime prompt, got %#v", got)
	}
}

func TestAgentLoopServiceExecutesToolsAndContinuesGeneration(t *testing.T) {
	current := testSession()
	call := domainmessage.ToolCall{ID: "call-1", Type: "function", Name: "read_file", Arguments: `{"path":"main.go"}`}
	model := &scriptedModel{
		results: []modelport.ChatResult{
			{
				Reasoning: "first",
				Usage:     modelport.TokenUsage{PromptTokens: 4, TotalTokens: 4, Available: true},
				ToolCalls: []domainmessage.ToolCall{call},
			},
			{
				Content:   "tool result handled",
				Reasoning: "second",
				Usage:     modelport.TokenUsage{CompletionTokens: 6, TotalTokens: 6, Available: true},
			},
		},
	}
	contexts := &recordingContextProvider{}
	tools := &recordingToolCatalog{}
	runtime := &recordingRuntimeProvider{prompt: "runtime-after-tool"}
	records := &recordingToolExecutionRecordSink{}
	executor := &recordingToolExecutor{
		result: ToolExecutionResult{
			Messages: []domainmessage.Message{
				domainmessage.ToolResultMessage(domainmessage.ToolResult{
					ToolCallID: "call-1",
					Name:       "read_file",
					Content:    "file content",
				}),
			},
			Entries: []domaintool.ExecutionEntry{{ToolCallID: "call-1"}},
			Assets:  []domaintool.SharedAsset{{ShortCode: "asset-code"}},
		},
	}

	result, err := AgentLoopService{
		Contexts:            contexts,
		Tools:               tools,
		RuntimeInstructions: runtime,
		ToolExecutor:        executor,
		ToolRecords:         records,
	}.Run(context.Background(), RunCommand{
		Model:         model,
		Session:       current,
		RuntimePrompt: "runtime-initial",
		LatestInput:   "inspect file",
		RequestID:     "request-1",
		ForceChatMode: true,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if result.Content != "tool result handled" {
		t.Fatalf("expected final content, got %q", result.Content)
	}
	if result.Reasoning != "first\nsecond" {
		t.Fatalf("expected joined reasoning, got %q", result.Reasoning)
	}
	if result.Usage.PromptTokens != 4 || result.Usage.CompletionTokens != 6 || result.Usage.TotalTokens != 10 {
		t.Fatalf("expected usage to be accumulated, got %#v", result.Usage)
	}
	if len(current.Messages) != 4 {
		t.Fatalf("expected assistant tool call and tool result messages, got %d messages", len(current.Messages))
	}
	if !current.Messages[2].HasToolCall() {
		t.Fatalf("expected assistant tool call message at index 2, got %#v", current.Messages[2])
	}
	if resultMessage, ok := current.Messages[3].FirstToolResult(); !ok || resultMessage.Content != "file content" {
		t.Fatalf("expected tool result message at index 3, got %#v", current.Messages[3])
	}
	if executor.calls != 1 || executor.last.RequestID != "request-1" || len(executor.last.Calls) != 1 {
		t.Fatalf("expected one tool execution command, got calls=%d command=%#v", executor.calls, executor.last)
	}
	if len(model.requests) != 2 || len(model.requests[0].Tools) != 1 || len(model.requests[1].Tools) != 1 {
		t.Fatalf("expected both loop requests to include tools, got %#v", model.requests)
	}
	if got := contexts.prompts; len(got) != 2 || got[0] != "runtime-initial" || got[1] != "runtime-after-tool" {
		t.Fatalf("expected runtime prompt refresh after tool execution, got %#v", got)
	}
	if runtime.input != "inspect file" || !runtime.forceChatMode {
		t.Fatalf("expected runtime provider to receive latest input and forced chat mode, got %#v", runtime)
	}
	if records.calls != 1 || len(records.last.Entries) != 1 || records.last.Entries[0].ToolCallID != "call-1" {
		t.Fatalf("expected tool execution entries to be recorded, got calls=%d command=%#v", records.calls, records.last)
	}
	if len(records.last.Assets) != 1 || records.last.Assets[0].ShortCode != "asset-code" {
		t.Fatalf("expected shared assets to be recorded, got %#v", records.last.Assets)
	}
}

func TestAgentLoopServiceSkipsToolRecordSinkWhenResultHasNoRecords(t *testing.T) {
	records := &recordingToolExecutionRecordSink{}

	AgentLoopService{ToolRecords: records}.RecordToolExecution(context.Background(), ToolExecutionResult{
		Messages: []domainmessage.Message{
			domainmessage.ToolResultMessage(domainmessage.ToolResult{
				ToolCallID: "call-1",
				Name:       "read_file",
				Content:    "file content",
			}),
		},
	})

	if records.calls != 0 {
		t.Fatalf("expected empty tool record result to be ignored, got calls=%d", records.calls)
	}
}

func TestAgentLoopServiceFinalGenerationOmitsToolsAfterMaxRounds(t *testing.T) {
	current := testSession()
	call := domainmessage.ToolCall{ID: "call-1", Type: "function", Name: "read_file", Arguments: `{}`}
	model := &scriptedModel{
		results: []modelport.ChatResult{
			{
				Reasoning: "tool round",
				Usage:     modelport.TokenUsage{PromptTokens: 1, TotalTokens: 1, Available: true},
				ToolCalls: []domainmessage.ToolCall{call},
			},
			{
				Content:   "final without tools",
				Reasoning: "final round",
				Usage:     modelport.TokenUsage{CompletionTokens: 2, TotalTokens: 2, Available: true},
			},
		},
	}
	contexts := &recordingContextProvider{}
	runtime := &recordingRuntimeProvider{prompt: "runtime-after-tool"}
	executor := &recordingToolExecutor{
		result: ToolExecutionResult{
			Messages: []domainmessage.Message{
				domainmessage.ToolResultMessage(domainmessage.ToolResult{ToolCallID: "call-1", Name: "read_file", Content: "ok"}),
			},
		},
	}

	result, err := AgentLoopService{
		Contexts:            contexts,
		Tools:               &recordingToolCatalog{},
		RuntimeInstructions: runtime,
		ToolExecutor:        executor,
		MaxToolRounds:       1,
	}.Run(context.Background(), RunCommand{
		Model:         model,
		Session:       current,
		RuntimePrompt: "runtime-initial",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if result.Content != "final without tools" {
		t.Fatalf("expected final no-tool generation, got %q", result.Content)
	}
	if result.Reasoning != "tool round\nfinal round" {
		t.Fatalf("expected reasoning from both rounds, got %q", result.Reasoning)
	}
	if len(model.requests) != 2 {
		t.Fatalf("expected tool round and final round requests, got %d", len(model.requests))
	}
	if len(model.requests[0].Tools) != 1 {
		t.Fatalf("expected first request to expose tools, got %#v", model.requests[0].Tools)
	}
	if len(model.requests[1].Tools) != 0 {
		t.Fatalf("expected final request to omit tools, got %#v", model.requests[1].Tools)
	}
	if got := contexts.prompts; len(got) != 2 || got[1] != "runtime-after-tool" {
		t.Fatalf("expected final request to use refreshed runtime prompt, got %#v", got)
	}
}

type scriptedModel struct {
	requests []modelport.GenerateRequest
	results  []modelport.ChatResult
	err      error
}

func (m *scriptedModel) Generate(ctx context.Context, request modelport.GenerateRequest) (modelport.ChatResult, error) {
	m.requests = append(m.requests, request)
	if m.err != nil {
		return modelport.ChatResult{}, m.err
	}
	if len(m.results) == 0 {
		return modelport.ChatResult{}, errors.New("unexpected generate call")
	}
	result := m.results[0]
	m.results = m.results[1:]
	return result, nil
}

type recordingContextProvider struct {
	prompts []string
}

func (p *recordingContextProvider) Snapshot(current *session.Session, runtimePrompt string) contextmgr.Snapshot {
	p.prompts = append(p.prompts, runtimePrompt)
	return contextmgr.Snapshot{Messages: domainmessage.CloneMessages(current.Messages)}
}

type recordingToolCatalog struct{}

func (recordingToolCatalog) ToolsForSession(current *session.Session, forceChatMode bool) []modelport.Tool {
	return []modelport.Tool{
		{
			Type: "function",
			Function: &modelport.FunctionDefinition{
				Name:        "read_file",
				Description: "Read a file",
			},
		},
	}
}

type recordingRuntimeProvider struct {
	prompt        string
	input         string
	forceChatMode bool
}

func (p *recordingRuntimeProvider) Prompt(ctx context.Context, current *session.Session, input string, forceChatMode bool) string {
	p.input = input
	p.forceChatMode = forceChatMode
	return p.prompt
}

type recordingToolExecutor struct {
	calls  int
	last   ToolExecutionCommand
	result ToolExecutionResult
	err    error
}

func (e *recordingToolExecutor) Execute(ctx context.Context, command ToolExecutionCommand) (ToolExecutionResult, error) {
	e.calls++
	e.last = command
	if e.err != nil {
		return ToolExecutionResult{}, e.err
	}
	return e.result, nil
}

type recordingToolExecutionRecordSink struct {
	calls int
	last  ToolExecutionRecordCommand
}

func (s *recordingToolExecutionRecordSink) RecordToolExecution(ctx context.Context, command ToolExecutionRecordCommand) {
	s.calls++
	s.last = command
}

func testSession() *session.Session {
	return &session.Session{
		ID: "session-1",
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleSystem, "system"),
			domainmessage.Text(domainmessage.RoleUser, "hello"),
		},
	}
}
