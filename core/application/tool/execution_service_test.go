package toolapp

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
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

func (t fakeExecutableTool) Call(context.Context, json.RawMessage) (tooldef.ToolOutput, error) {
	return tooldef.SuccessOutput(t.result), t.err
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
			OnToolResult: func(name string, arguments string, output domaintool.ToolOutput) {
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
	if result.Entries[1].Status != domaintool.ResultStatusDenied || result.Entries[1].ErrorCode != "permission_denied" {
		t.Fatalf("expected structured permission denial: %#v", result.Entries[1])
	}
	toolResult, ok := result.Messages[0].FirstToolResult()
	if !ok || toolResult.Content == "" {
		t.Fatalf("expected model-visible permission denial message: %#v", result.Messages[0])
	}
}

func TestExecutionServiceHookAllowStillAsksForPermission(t *testing.T) {
	asked := false
	result, err := (ExecutionService{
		Registry: fakeRegistry{tools: map[string]tooldef.Tool{
			"write_file": fakeExecutableTool{name: "write_file", permission: tooldef.PermissionWrite, result: "written"},
		}},
		Hooks: &fakeHookBridge{before: HookResult{Decision: HookDecisionAllow}},
	}).Execute(context.Background(), ExecutionCommand{
		SessionID:      "session-1",
		PermissionMode: session.PermissionModeAsk,
		Calls:          []domainmessage.ToolCall{{ID: "call-1", Name: "write_file"}},
		Callbacks: ExecutionCallbacks{OnToolAsk: func(PermissionRequest) bool {
			asked = true
			return false
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !asked || result.Entries[1].Status != domaintool.ResultStatusDenied {
		t.Fatalf("expected hook allow to continue through permission policy: %#v", result)
	}
}

func TestExecutionServiceHookAskRequiresConfirmationInFullMode(t *testing.T) {
	asked := false
	executed := false
	result, err := (ExecutionService{
		Registry: fakeRegistry{tools: map[string]tooldef.Tool{
			"write_file": recordingExecutableTool{onCall: func() { executed = true }},
		}},
		Hooks: &fakeHookBridge{before: HookResult{Decision: HookDecisionAsk}},
	}).Execute(context.Background(), ExecutionCommand{
		SessionID: "session-1", PermissionMode: session.PermissionModeFull,
		Calls: []domainmessage.ToolCall{{ID: "call-1", Name: "write_file"}},
		Callbacks: ExecutionCallbacks{OnToolAsk: func(PermissionRequest) bool {
			asked = true
			return false
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !asked || executed || result.Entries[1].ErrorCode != "permission_denied" {
		t.Fatalf("expected hook ask to require confirmation: %#v", result)
	}
}

func TestExecutionServiceReturnsPartialResultAfterLaterLookupFailure(t *testing.T) {
	executed := false
	result, err := (ExecutionService{
		Registry: fakeRegistry{tools: map[string]tooldef.Tool{
			"write_file": recordingExecutableTool{onCall: func() { executed = true }},
		}},
	}).Execute(context.Background(), ExecutionCommand{
		SessionID: "session-1", PermissionMode: session.PermissionModeFull,
		Calls: []domainmessage.ToolCall{
			{ID: "call-1", Name: "write_file", Arguments: `{"path":"a.txt"}`},
			{ID: "call-2", Name: "missing_tool", Arguments: `{}`},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !executed || len(result.Calls) != 2 || len(result.Messages) != 2 || len(result.Entries) != 4 {
		t.Fatalf("expected completed call and failed lookup to be preserved: %#v", result)
	}
	if result.Entries[3].ErrorCode != "tool_not_found" {
		t.Fatalf("expected missing-tool record, got %#v", result.Entries[3])
	}
}

func TestExecutionServiceUsesHookRewrittenArgumentsForSessionCall(t *testing.T) {
	arguments := ""
	result, err := (ExecutionService{
		Registry: fakeRegistry{tools: map[string]tooldef.Tool{
			"write_file": argumentRecordingTool{onCall: func(value string) { arguments = value }},
		}},
		Hooks: &fakeHookBridge{before: HookResult{Decision: HookDecisionContinue, Arguments: `{"path":"rewritten.txt"}`}},
	}).Execute(context.Background(), ExecutionCommand{
		SessionID: "session-1", PermissionMode: session.PermissionModeFull,
		Calls: []domainmessage.ToolCall{{ID: "call-1", Name: "write_file", Arguments: `{"path":"original.txt"}`}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if arguments != `{"path":"rewritten.txt"}` || result.Calls[0].Arguments != arguments || result.Entries[0].Arguments != arguments {
		t.Fatalf("expected rewritten arguments throughout execution result: %#v", result)
	}
}

func TestExecutionServicePlanModeBlocksWriteEvenWithFullPermission(t *testing.T) {
	executed := false
	result, err := (ExecutionService{
		Registry: fakeRegistry{tools: map[string]tooldef.Tool{
			"write_file": recordingExecutableTool{onCall: func() { executed = true }},
		}},
	}).Execute(context.Background(), ExecutionCommand{
		SessionID:      "session-1",
		AgentMode:      session.AgentModePlan,
		PermissionMode: session.PermissionModeFull,
		Calls:          []domainmessage.ToolCall{{ID: "call-1", Name: "write_file"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if executed || result.Entries[1].ErrorCode != "plan_mode_denied" {
		t.Fatalf("expected plan hard gate before execution: %#v", result)
	}
}

func TestExecutionServiceRejectsToolOutsideEnforcedAllowlist(t *testing.T) {
	executed := false
	result, err := (ExecutionService{
		Registry: fakeRegistry{tools: map[string]tooldef.Tool{
			"write_file": recordingExecutableTool{onCall: func() { executed = true }},
		}},
	}).Execute(context.Background(), ExecutionCommand{
		SessionID:            "subagent-session-1",
		PermissionMode:       session.PermissionModeFull,
		AllowedTools:         []string{},
		EnforceToolAllowlist: true,
		Calls:                []domainmessage.ToolCall{{ID: "call-1", Name: "write_file"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if executed {
		t.Fatal("expected execution allowlist to block the tool before invocation")
	}
	if len(result.Entries) != 2 || result.Entries[1].Status != domaintool.ResultStatusDenied || result.Entries[1].ErrorCode != "subagent_tool_denied" {
		t.Fatalf("expected structured subagent allowlist denial, got %#v", result)
	}
}

func TestExecutionServiceRunsReadToolsInParallel(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan struct{})
	var result ExecutionResult
	var executeErr error
	go func() {
		defer close(done)
		result, executeErr = (ExecutionService{
			Registry: fakeRegistry{tools: map[string]tooldef.Tool{
				"read_file": blockingTool{name: "read_file", permission: tooldef.PermissionRead, started: started, release: release, result: "ok"},
			}},
		}).Execute(context.Background(), ExecutionCommand{
			SessionID:      "session-1",
			PermissionMode: session.PermissionModeAsk,
			Calls: []domainmessage.ToolCall{
				{ID: "call-1", Name: "read_file", Arguments: `{"path":"a.go"}`},
				{ID: "call-2", Name: "read_file", Arguments: `{"path":"b.go"}`},
			},
		})
	}()
	waitStarted(t, started, 2)
	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for parallel reads")
	}
	if executeErr != nil {
		t.Fatal(executeErr)
	}
	if len(result.Calls) != 2 || result.Calls[0].ID != "call-1" || result.Calls[1].ID != "call-2" {
		t.Fatalf("expected original call order, got %#v", result.Calls)
	}
}

func TestExecutionServiceKeepsCallOrderWhenReadsFinishOutOfOrder(t *testing.T) {
	slowRelease := make(chan struct{})
	fastRelease := make(chan struct{})
	close(fastRelease)
	go func() {
		time.Sleep(40 * time.Millisecond)
		close(slowRelease)
	}()
	result, err := (ExecutionService{
		Registry: fakeRegistry{tools: map[string]tooldef.Tool{
			"slow_read": blockingTool{name: "slow_read", permission: tooldef.PermissionRead, release: slowRelease, result: "slow"},
			"fast_read": blockingTool{name: "fast_read", permission: tooldef.PermissionRead, release: fastRelease, result: "fast"},
		}},
	}).Execute(context.Background(), ExecutionCommand{
		SessionID:      "session-1",
		PermissionMode: session.PermissionModeAsk,
		Calls: []domainmessage.ToolCall{
			{ID: "call-1", Name: "slow_read"},
			{ID: "call-2", Name: "fast_read"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Messages) != 2 {
		t.Fatalf("expected two results, got %#v", result.Messages)
	}
	first, _ := result.Messages[0].FirstToolResult()
	second, _ := result.Messages[1].FirstToolResult()
	if first.Content != "slow" || second.Content != "fast" {
		t.Fatalf("expected slow then fast in original order, got %#v %#v", first, second)
	}
}

func TestExecutionServiceSerializesWriteBehindRead(t *testing.T) {
	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32
	track := func() {
		current := concurrent.Add(1)
		for {
			observed := maxConcurrent.Load()
			if current <= observed || maxConcurrent.CompareAndSwap(observed, current) {
				break
			}
		}
		time.Sleep(40 * time.Millisecond)
		concurrent.Add(-1)
	}
	_, err := (ExecutionService{
		Registry: fakeRegistry{tools: map[string]tooldef.Tool{
			"read_file":  countingTool{name: "read_file", permission: tooldef.PermissionRead, onCall: track, result: "ok"},
			"write_file": countingTool{name: "write_file", permission: tooldef.PermissionWrite, onCall: track, result: "written"},
		}},
	}).Execute(context.Background(), ExecutionCommand{
		SessionID:      "session-1",
		PermissionMode: session.PermissionModeFull,
		Calls: []domainmessage.ToolCall{
			{ID: "call-1", Name: "read_file", Arguments: `{"path":"a.go"}`},
			{ID: "call-2", Name: "write_file", Arguments: `{"path":"b.go"}`},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if maxConcurrent.Load() != 1 {
		t.Fatalf("read and write must not overlap, max concurrent = %d", maxConcurrent.Load())
	}
}

func TestExecutionServiceAllowsGlobalToolForOrdinarySession(t *testing.T) {
	executed := false
	result, err := (ExecutionService{
		Registry: fakeRegistry{tools: map[string]tooldef.Tool{
			"write_file": recordingExecutableTool{onCall: func() { executed = true }},
		}},
	}).Execute(context.Background(), ExecutionCommand{
		SessionID:      "session-1",
		PermissionMode: session.PermissionModeFull,
		AllowedTools:   nil,
		Calls:          []domainmessage.ToolCall{{ID: "call-1", Name: "write_file"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !executed || len(result.Entries) != 2 || result.Entries[1].Status != domaintool.ResultStatusSuccess {
		t.Fatalf("expected ordinary session to execute globally registered tool, got %#v", result)
	}
}

type recordingExecutableTool struct {
	onCall func()
}

type argumentRecordingTool struct {
	onCall func(string)
}

func (argumentRecordingTool) Name() string                   { return "write_file" }
func (argumentRecordingTool) Description() string            { return "write" }
func (argumentRecordingTool) Schema() any                    { return nil }
func (argumentRecordingTool) Permission() tooldef.Permission { return tooldef.PermissionWrite }
func (t argumentRecordingTool) Call(_ context.Context, arguments json.RawMessage) (tooldef.ToolOutput, error) {
	if t.onCall != nil {
		t.onCall(string(arguments))
	}
	return tooldef.SuccessOutput("written"), nil
}

func (recordingExecutableTool) Name() string                   { return "write_file" }
func (recordingExecutableTool) Description() string            { return "write" }
func (recordingExecutableTool) Schema() any                    { return nil }
func (recordingExecutableTool) Permission() tooldef.Permission { return tooldef.PermissionWrite }
func (t recordingExecutableTool) Call(context.Context, json.RawMessage) (tooldef.ToolOutput, error) {
	if t.onCall != nil {
		t.onCall()
	}
	return tooldef.SuccessOutput("written"), nil
}

type blockingTool struct {
	name       string
	permission tooldef.Permission
	started    chan struct{}
	release    chan struct{}
	result     string
}

func (t blockingTool) Name() string                   { return t.name }
func (t blockingTool) Description() string            { return t.name }
func (t blockingTool) Schema() any                    { return nil }
func (t blockingTool) Permission() tooldef.Permission { return t.permission }
func (t blockingTool) Call(ctx context.Context, _ json.RawMessage) (tooldef.ToolOutput, error) {
	if t.started != nil {
		select {
		case t.started <- struct{}{}:
		default:
		}
	}
	if t.release != nil {
		select {
		case <-t.release:
		case <-ctx.Done():
			return tooldef.ToolOutput{}, ctx.Err()
		}
	}
	return tooldef.SuccessOutput(t.result), nil
}

func waitStarted(t *testing.T, started <-chan struct{}, count int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for i := 0; i < count; i++ {
		select {
		case <-started:
		case <-deadline:
			t.Fatalf("timed out waiting for %d overlapping tool starts", count)
		}
	}
}

type countingTool struct {
	name       string
	permission tooldef.Permission
	onCall     func()
	result     string
}

func (t countingTool) Name() string                   { return t.name }
func (t countingTool) Description() string            { return t.name }
func (t countingTool) Schema() any                    { return nil }
func (t countingTool) Permission() tooldef.Permission { return t.permission }
func (t countingTool) Call(context.Context, json.RawMessage) (tooldef.ToolOutput, error) {
	if t.onCall != nil {
		t.onCall()
	}
	return tooldef.SuccessOutput(t.result), nil
}
