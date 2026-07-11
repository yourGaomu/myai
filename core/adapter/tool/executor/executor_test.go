package executor

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	generationcommand "myai/core/application/chat/generation/command"
	toolcommand "myai/core/application/tool/command"
	domainmessage "myai/core/domain/message"
	"myai/core/hook"
	modelport "myai/core/port/model"
	"myai/core/session"
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

func (t fakeTool) Call(context.Context, json.RawMessage) (string, error) {
	return t.result, nil
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
