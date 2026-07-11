package toolapp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	domainmessage "myai/core/domain/message"
	"myai/core/session"
	tooldef "myai/core/tool/tool"
)

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

type fakeExecutableTool struct {
	name       string
	permission tooldef.Permission
	result     string
	err        error
}

func (t fakeExecutableTool) Name() string {
	return t.name
}

func (t fakeExecutableTool) Description() string {
	return t.name
}

func (t fakeExecutableTool) Schema() any {
	return nil
}

func (t fakeExecutableTool) Permission() tooldef.Permission {
	return t.permission
}

func (t fakeExecutableTool) Call(context.Context, json.RawMessage) (string, error) {
	return t.result, t.err
}

type fakeHookBridge struct {
	before HookResult
	after  []HookEvent
}

func (h *fakeHookBridge) BeforeToolUse(context.Context, HookEvent) (HookResult, error) {
	return h.before, nil
}

func (h *fakeHookBridge) AfterToolUse(_ context.Context, event HookEvent) {
	h.after = append(h.after, event)
}

func TestExecutionServiceExecutesTool(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	hooks := &fakeHookBridge{before: HookResult{Decision: HookDecisionContinue}}
	var calledName string
	var resultName string

	result, err := (ExecutionService{
		Registry: fakeRegistry{tools: map[string]tooldef.Tool{
			"read_file": fakeExecutableTool{name: "read_file", permission: tooldef.PermissionRead, result: "content"},
		}},
		Hooks: hooks,
		Now:   func() time.Time { return now },
	}).Execute(context.Background(), ExecutionCommand{
		SessionID:      "session-1",
		PermissionMode: session.PermissionModeReadonly,
		Calls: []domainmessage.ToolCall{{
			ID:        "call-1",
			Name:      "read_file",
			Arguments: `{"path":"README.md"}`,
		}},
		Callbacks: ExecutionCallbacks{
			OnToolCall: func(name string, arguments string) {
				calledName = name
			},
			OnToolResult: func(name string, arguments string, result string) {
				resultName = name
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calledName != "read_file" || resultName != "read_file" {
		t.Fatalf("expected callbacks, got %q %q", calledName, resultName)
	}
	if len(result.Messages) != 1 || len(result.Entries) != 2 {
		t.Fatalf("unexpected execution result: %#v", result)
	}
	if len(hooks.after) != 1 || hooks.after[0].Result != "content" {
		t.Fatalf("unexpected hook events: %#v", hooks.after)
	}
}

func TestExecutionServiceHonorsHookDeny(t *testing.T) {
	result, err := (ExecutionService{
		Registry: fakeRegistry{tools: map[string]tooldef.Tool{
			"write_file": fakeExecutableTool{name: "write_file", permission: tooldef.PermissionWrite, result: "written"},
		}},
		Hooks: &fakeHookBridge{before: HookResult{Decision: HookDecisionDeny, Message: "blocked"}},
	}).Execute(context.Background(), ExecutionCommand{
		SessionID:      "session-1",
		PermissionMode: session.PermissionModeFull,
		Calls: []domainmessage.ToolCall{{
			ID:   "call-1",
			Name: "write_file",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("unexpected entries: %#v", result.Entries)
	}
	if result.Entries[1].Error == "" {
		t.Fatalf("expected denied tool error: %#v", result.Entries[1])
	}
}

func TestExecutionServiceHonorsAskDenial(t *testing.T) {
	result, err := (ExecutionService{
		Registry: fakeRegistry{tools: map[string]tooldef.Tool{
			"write_file": fakeExecutableTool{name: "write_file", permission: tooldef.PermissionWrite, result: "written"},
		}},
	}).Execute(context.Background(), ExecutionCommand{
		SessionID:      "session-1",
		PermissionMode: session.PermissionModeAsk,
		Calls: []domainmessage.ToolCall{{
			ID:   "call-1",
			Name: "write_file",
		}},
		Callbacks: ExecutionCallbacks{
			OnToolAsk: func(PermissionRequest) bool {
				return false
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("unexpected entries: %#v", result.Entries)
	}
	if result.Entries[1].Error != "" {
		t.Fatalf("permission denial is a model-visible result, not tool error: %#v", result.Entries[1])
	}
	toolResult, ok := result.Messages[0].FirstToolResult()
	if !ok || toolResult.Content == "" {
		t.Fatalf("expected model-visible permission denial message: %#v", result.Messages[0])
	}
}
