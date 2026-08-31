package executor

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	agentrunruntime "myai/core/application/agentrun/runtime"
	generationcommand "myai/core/application/chat/generation/command"
	toolcommand "myai/core/application/tool/command"
	"myai/core/contextmgr"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	"myai/core/hook"
	modelport "myai/core/port/model"
	"myai/core/session"
	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

func TestExecutorMapsStreamCallbacksAndReturnsExecutionRecords(t *testing.T) {
	var asked modelport.ToolPermissionRequest
	executor := Executor{
		Registry: fakeRegistry{tools: map[string]tooldef.Tool{
			"write_file": fakeTool{name: "write_file", permission: tooldef.PermissionWrite, result: "written"},
		}},
	}

	result, err := executor.Execute(context.Background(), toolCommand(modelport.ChatStreamHandler{
		OnToolAsk: func(request modelport.ToolPermissionRequest) bool {
			asked = request
			return true
		},
	}))
	if err != nil {
		t.Fatal(err)
	}

	if asked.Name != "write_file" || asked.Permission != tooldef.PermissionWrite || asked.Mode != string(session.PermissionModeAsk) {
		t.Fatalf("unexpected permission request: %#v", asked)
	}
	if len(result.Messages) != 1 || len(result.Entries) != 2 {
		t.Fatalf("unexpected tool execution result: %#v", result)
	}
}

func TestExecutorRejectsNilSession(t *testing.T) {
	_, err := (Executor{}).Execute(context.Background(), generationcommand.ToolExecution{})
	if err == nil {
		t.Fatal("expected nil session error")
	}
}

func TestExecutorPreservesCurrentTaskIDForNestedSubagents(t *testing.T) {
	var execution toolruntime.Execution
	executor := Executor{
		Registry: fakeRegistry{tools: map[string]tooldef.Tool{
			"inspect_execution": fakeTool{
				name:       "inspect_execution",
				permission: tooldef.PermissionRead,
				inspect: func(ctx context.Context) {
					execution, _ = toolruntime.CurrentExecution(ctx)
				},
			},
		}},
	}
	session := &session.Session{
		ID: "child-session", Kind: session.KindSubagent, ParentTaskID: "parent-task",
		PermissionMode: session.PermissionModeReadonly,
	}
	ctx := agentrunruntime.WithMetadata(context.Background(), agentrunruntime.Metadata{
		TaskID: "current-task", ParentRunID: "parent-run", PlanID: "plan-1", StepID: "step-1",
	})
	_, err := executor.Execute(ctx, generationcommand.ToolExecution{
		Session: session,
		Calls:   []domainmessage.ToolCall{{ID: "call-1", Name: "inspect_execution"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if execution.TaskID != "current-task" {
		t.Fatalf("tool execution task id = %q, want current task id", execution.TaskID)
	}
	if execution.TaskID == session.ParentTaskID {
		t.Fatal("tool execution must not expose the session parent task as the current task")
	}
}

func TestLimitToolResultsForPromptKeepsFullAuditEntries(t *testing.T) {
	firstContent := strings.Repeat("first output ", 3000)
	secondContent := strings.Repeat("second output ", 3000)
	messages := []domainmessage.Message{
		domainmessage.ToolResultMessage(domainmessage.ToolResult{ToolCallID: "call-1", Name: "read_file", Content: firstContent}),
		domainmessage.ToolResultMessage(domainmessage.ToolResult{ToolCallID: "call-2", Name: "read_file", Content: secondContent}),
	}
	entries := []domaintool.ExecutionEntry{
		{Kind: domaintool.ExecutionEntryToolResult, ToolCallID: "call-1", Content: firstContent},
		{Kind: domaintool.ExecutionEntryToolResult, ToolCallID: "call-2", Content: secondContent},
	}

	limited, annotated := limitToolResultsForPrompt(messages, entries, 500)
	if tokens := contextmgr.EstimateMessagesTokens(limited); tokens > 500 {
		t.Fatalf("limited tool results use %d tokens, want at most 500", tokens)
	}
	for index, message := range limited {
		result, ok := message.FirstToolResult()
		if !ok || !result.PromptTruncated || result.FullContent == "" {
			t.Fatalf("expected limited result %d to be marked truncated: %#v", index, message)
		}
	}
	if annotated[0].Content != firstContent || annotated[1].Content != secondContent {
		t.Fatal("full audit content was modified")
	}
	if !annotated[0].PromptTruncated || annotated[0].PromptContent == "" || annotated[0].PromptContent == firstContent {
		t.Fatalf("expected separate bounded prompt content: %#v", annotated[0])
	}
}

func TestPromptToolResultBudgetShrinksAsCurrentTurnGrows(t *testing.T) {
	current := &session.Session{
		ContextWindowK: 4,
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleSystem, "system"),
			domainmessage.Text(domainmessage.RoleUser, "inspect the project"),
		},
	}
	calls := []domainmessage.ToolCall{{ID: "call-1", Name: "read_file"}}
	before := promptToolResultBudget(current, calls)
	current.Messages = append(current.Messages,
		domainmessage.ToolCallMessage(calls),
		domainmessage.ToolResultMessage(domainmessage.ToolResult{
			ToolCallID: "call-1", Name: "read_file", Content: strings.Repeat("x", 4000),
		}),
	)
	after := promptToolResultBudget(current, calls)
	if before <= 0 || after >= before {
		t.Fatalf("expected current-turn growth to reduce budget, before=%d after=%d", before, after)
	}
}

func TestHookBridgeMapsBeforeAndAfterToolUse(t *testing.T) {
	handler := &recordingHookHandler{
		result: hook.Result{
			Decision:  hook.DecisionAllow,
			Arguments: `{"path":"README.md"}`,
			Message:   "ok",
		},
	}
	manager := &hook.Manager{}
	manager.Register(handler)

	bridge := HookBridge{Hooks: manager}
	result, err := bridge.BeforeToolUse(context.Background(), fakeHookEvent())
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != "allow" || result.Arguments == "" || result.Message != "ok" {
		t.Fatalf("unexpected before hook result: %#v", result)
	}

	bridge.AfterToolUse(context.Background(), fakeHookEvent())
	if len(handler.events) != 2 || handler.events[0].Type != hook.EventPreToolUse || handler.events[1].Type != hook.EventPostToolUse {
		t.Fatalf("unexpected hook events: %#v", handler.events)
	}
}

func TestHookBridgeReportsPostHookError(t *testing.T) {
	manager := &hook.Manager{}
	manager.Register(&recordingHookHandler{err: errors.New("post failed")})
	var reported error

	HookBridge{
		Hooks: manager,
		OnPostError: func(err error) {
			reported = err
		},
	}.AfterToolUse(context.Background(), fakeHookEvent())

	if reported == nil {
		t.Fatal("expected post hook error to be reported")
	}
}

func toolCommand(stream modelport.ChatStreamHandler) generationcommand.ToolExecution {
	return generationcommand.ToolExecution{
		Session: &session.Session{
			ID:             "session-1",
			PermissionMode: session.PermissionModeAsk,
		},
		Calls: []domainmessage.ToolCall{{
			ID:        "call-1",
			Name:      "write_file",
			Arguments: `{"path":"README.md"}`,
		}},
		Stream: stream,
	}
}

type fakeRegistry struct {
	tools map[string]tooldef.Tool
}

func (r fakeRegistry) GetTool(name string) (tooldef.Tool, error) {
	tool := r.tools[name]
	if tool == nil {
		return nil, errors.New("tool not found")
	}
	return tool, nil
}

type fakeTool struct {
	name       string
	permission tooldef.Permission
	result     string
	inspect    func(context.Context)
}

func (t fakeTool) Name() string {
	return t.name
}

func (t fakeTool) Description() string {
	return t.name
}

func (t fakeTool) Schema() any {
	return nil
}

func (t fakeTool) Permission() tooldef.Permission {
	return t.permission
}

func (t fakeTool) Call(ctx context.Context, _ json.RawMessage) (tooldef.ToolOutput, error) {
	if t.inspect != nil {
		t.inspect(ctx)
	}
	return tooldef.SuccessOutput(t.result), nil
}

type recordingHookHandler struct {
	result hook.Result
	err    error
	events []hook.Event
}

func (h *recordingHookHandler) HandleHook(ctx context.Context, event hook.Event) (hook.Result, error) {
	h.events = append(h.events, event)
	return h.result, h.err
}

func fakeHookEvent() toolcommand.HookEvent {
	return toolcommand.HookEvent{
		SessionID:  "session-1",
		Name:       "write_file",
		Arguments:  `{"path":"README.md"}`,
		Permission: tooldef.PermissionWrite,
		Result:     "written",
	}
}
