package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	toolcommand "myai/core/application/tool/command"
	toolresult "myai/core/application/tool/result"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	"myai/core/session"
	tooldef "myai/core/tool/tool"
)

type executionTestRegistry map[string]tooldef.Tool

func (registry executionTestRegistry) GetTool(name string) (tooldef.Tool, error) {
	if registered, ok := registry[name]; ok {
		return registered, nil
	}
	return nil, errors.New("tool not found")
}

type executionTestTool struct {
	name       string
	permission tooldef.Permission
	barrier    bool
	schema     any
	call       func(context.Context, json.RawMessage) (domaintool.ToolOutput, error)
}

func (tool executionTestTool) Name() string                   { return tool.name }
func (tool executionTestTool) Description() string            { return tool.name }
func (tool executionTestTool) Schema() any                    { return tool.schema }
func (tool executionTestTool) Permission() tooldef.Permission { return tool.permission }
func (tool executionTestTool) IsTurnBarrier() bool            { return tool.barrier }
func (tool executionTestTool) Call(ctx context.Context, args json.RawMessage) (domaintool.ToolOutput, error) {
	if tool.call != nil {
		return tool.call(ctx, args)
	}
	return domaintool.SuccessOutput(tool.name), nil
}

func TestExecutionPreservesModelOrderAcrossStateChangingCalls(t *testing.T) {
	for attempt := 0; attempt < 30; attempt++ {
		var calls, asks []string
		registry := executionTestRegistry{}
		names := []string{"write", "read", "execute", "unknown", "empty_permission", "missing", "last_write"}
		permissions := []tooldef.Permission{tooldef.PermissionWrite, tooldef.PermissionRead, tooldef.PermissionExecute, "unrecognized", "", "", tooldef.PermissionWrite}
		command := toolcommand.Execution{SessionID: "session", PermissionMode: session.PermissionModeAsk}
		for index, name := range names {
			if name != "missing" {
				registry[name] = executionTestTool{name: name, permission: permissions[index]}
			}
			command.Calls = append(command.Calls, domainmessage.ToolCall{ID: name, Name: name, Arguments: `{}`})
		}
		command.Callbacks = toolcommand.ExecutionCallbacks{
			OnToolCallIdentified: func(call domainmessage.ToolCall) { calls = append(calls, call.ID) },
			OnToolAsk: func(request toolcommand.PermissionRequest) bool {
				asks = append(asks, request.ToolCallID)
				return true
			},
		}
		result, err := (ExecutionService{Registry: registry}).Execute(context.Background(), command)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(calls, names) {
			t.Fatalf("execution events reordered: got %v, want %v", calls, names)
		}
		wantAsks := []string{"write", "execute", "last_write"}
		if !reflect.DeepEqual(asks, wantAsks) {
			t.Fatalf("approval events reordered: got %v, want %v", asks, wantAsks)
		}
		if len(result.Calls) != len(names) || len(result.Entries) != len(names)*2 {
			t.Fatalf("missing call/results: %#v", result)
		}
		for index, call := range result.Calls {
			if call.ID != names[index] || result.Entries[2*index+1].ToolCallID != call.ID {
				t.Fatalf("result identity/order changed at %d", index)
			}
		}
	}
}

func TestExecutionParallelizesOnlyConsecutiveReadGroups(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	started := make(chan string, 8)
	firstReads, write, lastReads := make(chan struct{}), make(chan struct{}), make(chan struct{})
	registry := executionTestRegistry{}
	command := toolcommand.Execution{SessionID: "session", PermissionMode: session.PermissionModeFull}
	for _, item := range []struct {
		name       string
		permission tooldef.Permission
		release    <-chan struct{}
	}{
		{"first_read_a", tooldef.PermissionRead, firstReads},
		{"first_read_b", tooldef.PermissionRead, firstReads},
		{"write", tooldef.PermissionWrite, write},
		{"last_read_a", tooldef.PermissionRead, lastReads},
		{"last_read_b", tooldef.PermissionRead, lastReads},
		{"execute", tooldef.PermissionExecute, nil},
	} {
		registry[item.name] = executionTestTool{name: item.name, permission: item.permission,
			call: func(ctx context.Context, _ json.RawMessage) (domaintool.ToolOutput, error) {
				started <- item.name
				if item.release != nil {
					select {
					case <-item.release:
					case <-ctx.Done():
						return domaintool.ToolOutput{}, ctx.Err()
					}
				}
				return domaintool.SuccessOutput(item.name), nil
			}}
		command.Calls = append(command.Calls, domainmessage.ToolCall{ID: item.name, Name: item.name, Arguments: `{}`})
	}
	done := make(chan error, 1)
	go func() {
		_, err := (ExecutionService{Registry: registry}).Execute(ctx, command)
		done <- err
	}()
	expectGroup := func(want ...string) {
		t.Helper()
		seen := map[string]bool{}
		for range want {
			select {
			case name := <-started:
				seen[name] = true
			case <-ctx.Done():
				t.Fatalf("calls did not overlap: want %v, saw %v", want, seen)
			}
		}
		for _, name := range want {
			if !seen[name] {
				t.Fatalf("call crossed read/write boundary: want %v, saw %v", want, seen)
			}
		}
		select {
		case name := <-started:
			t.Fatalf("%s started before preceding group completed", name)
		default:
		}
	}
	expectGroup("first_read_a", "first_read_b")
	close(firstReads)
	expectGroup("write")
	close(write)
	expectGroup("last_read_a", "last_read_b")
	close(lastReads)
	expectGroup("execute")
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("execution did not finish")
	}
}

func TestExecutionContainsToolPanicsAndCompletesResultLifecycle(t *testing.T) {
	for _, mode := range []string{"read", "write", "barrier", "nil_panic"} {
		t.Run(mode, func(t *testing.T) {
			var mu sync.Mutex
			var callCount, resultCount, afterCount int
			hooks := executionTestHooks{after: func(event toolcommand.HookEvent) {
				mu.Lock()
				defer mu.Unlock()
				afterCount++
				if event.Name == "panicking" && event.Err == nil {
					t.Error("post-tool Hook did not receive the panic failure")
				}
			}}
			permission := tooldef.PermissionRead
			if mode == "write" {
				permission = tooldef.PermissionWrite
			}
			panicking := executionTestTool{name: "panicking", permission: permission, barrier: mode == "barrier",
				call: func(context.Context, json.RawMessage) (domaintool.ToolOutput, error) {
					if mode == "nil_panic" {
						panic(nil)
					}
					panic("tool failure")
				}}
			command := toolcommand.Execution{SessionID: "session", PermissionMode: session.PermissionModeFull,
				Calls: []domainmessage.ToolCall{{ID: "panic-call", Name: "panicking", Arguments: `{}`}, {ID: "next-call", Name: "next", Arguments: `{}`}},
				Callbacks: toolcommand.ExecutionCallbacks{
					OnToolCallIdentified: func(domainmessage.ToolCall) { mu.Lock(); callCount++; mu.Unlock() },
					OnToolResultIdentified: func(domainmessage.ToolCall, domaintool.ToolOutput) {
						mu.Lock()
						resultCount++
						mu.Unlock()
					},
				}}
			result, err := (ExecutionService{Registry: executionTestRegistry{"panicking": panicking, "next": executionTestTool{name: "next", permission: tooldef.PermissionRead}}, Hooks: hooks}).Execute(context.Background(), command)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Messages) != 2 || len(result.Entries) != 4 || result.Entries[1].ErrorCode != "tool_panic" || result.Entries[1].Status != domaintool.ResultStatusFailed {
				t.Fatalf("panic did not become a structured result: %#v", result)
			}
			first, ok := result.Messages[0].FirstToolResult()
			if !ok || first.ToolCallID != "panic-call" || first.ErrorCode != "tool_panic" {
				t.Fatalf("panic result lost its call identity: %#v", first)
			}
			wantAfter := 2
			wantNext := domaintool.ResultStatusSuccess
			if mode == "barrier" {
				wantAfter = 1
				wantNext = domaintool.ResultStatusSkipped
			}
			if result.Entries[3].Status != wantNext || callCount != 2 || resultCount != 2 || afterCount != wantAfter {
				t.Fatalf("lifecycle changed: calls=%d results=%d hooks=%d entries=%#v", callCount, resultCount, afterCount, result.Entries)
			}
		})
	}
}

func TestExecutionEnforcesInheritedPermissionCeiling(t *testing.T) {
	for _, test := range []struct {
		name       string
		permission tooldef.Permission
		ceiling    []tooldef.Permission
		allowed    bool
	}{
		{"unrestricted", tooldef.PermissionExecute, nil, true},
		{"empty denies read", tooldef.PermissionRead, []tooldef.Permission{}, false},
		{"read cannot execute", tooldef.PermissionExecute, []tooldef.Permission{tooldef.PermissionRead}, false},
		{"read cannot write", tooldef.PermissionWrite, []tooldef.Permission{tooldef.PermissionRead}, false},
		{"read stays read", tooldef.PermissionRead, []tooldef.Permission{tooldef.PermissionRead}, true},
		{"write explicitly allowed", tooldef.PermissionWrite, []tooldef.Permission{tooldef.PermissionRead, tooldef.PermissionWrite}, true},
		{"execute excludes write", tooldef.PermissionWrite, []tooldef.Permission{tooldef.PermissionRead, tooldef.PermissionExecute}, false},
		{"unknown denied", "invalid", nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []session.PermissionMode{session.PermissionModeFull, session.PermissionModeAsk} {
				calls, asks := 0, 0
				tool := executionTestTool{name: "tool", permission: test.permission,
					call: func(context.Context, json.RawMessage) (domaintool.ToolOutput, error) {
						calls++
						return domaintool.SuccessOutput("done"), nil
					}}
				result, err := (ExecutionService{Registry: executionTestRegistry{"tool": tool}}).Execute(context.Background(), toolcommand.Execution{
					SessionID: "session", PermissionMode: mode, PermissionCeiling: test.ceiling,
					Calls:     []domainmessage.ToolCall{{ID: "call", Name: "tool", Arguments: `{}`}},
					Callbacks: toolcommand.ExecutionCallbacks{OnToolAsk: func(toolcommand.PermissionRequest) bool { asks++; return true }},
				})
				if err != nil {
					t.Fatal(err)
				}
				if test.allowed {
					if calls != 1 || result.Entries[1].Status != domaintool.ResultStatusSuccess {
						t.Fatalf("mode %s denied allowed capability: %#v", mode, result)
					}
				} else if calls != 0 || asks != 0 || result.Entries[1].ErrorCode != "permission_ceiling_denied" {
					t.Fatalf("mode %s bypassed inherited capability limit: calls=%d asks=%d result=%#v", mode, calls, asks, result)
				}
			}
		})
	}
}

func TestExecutionDoesNotRecoverPersistenceCallbackPanics(t *testing.T) {
	const failure = "persistence callback failed"
	defer func() {
		if recovered := recover(); recovered != failure {
			t.Fatalf("internal panic was swallowed or changed: %v", recovered)
		}
	}()
	_, _ = (ExecutionService{Registry: executionTestRegistry{"write": executionTestTool{name: "write", permission: tooldef.PermissionWrite}}}).Execute(context.Background(), toolcommand.Execution{
		SessionID: "session", PermissionMode: session.PermissionModeFull,
		Calls:     []domainmessage.ToolCall{{ID: "call", Name: "write", Arguments: `{}`}},
		Callbacks: toolcommand.ExecutionCallbacks{OnToolResult: func(string, string, domaintool.ToolOutput) { panic(failure) }},
	})
}

type executionTestHooks struct {
	arguments string
	after     func(toolcommand.HookEvent)
}

func (hooks executionTestHooks) BeforeToolUse(context.Context, toolcommand.HookEvent) (toolresult.Hook, error) {
	return toolresult.Hook{Decision: toolresult.HookDecisionContinue, Arguments: hooks.arguments}, nil
}
func (hooks executionTestHooks) AfterToolUse(_ context.Context, event toolcommand.HookEvent) {
	if hooks.after != nil {
		hooks.after(event)
	}
}
