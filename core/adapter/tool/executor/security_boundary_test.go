package executor

import (
	"context"
	"testing"

	generationcommand "myai/core/application/chat/generation/command"
	toolapplicationruntime "myai/core/application/tool/runtime"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	modelport "myai/core/port/model"
	"myai/core/session"
	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

type attestedShell struct{ fakeTool }

func (t attestedShell) IsSandboxedShell(ctx context.Context) bool {
	execution, ok := toolruntime.CurrentExecution(ctx)
	return ok && execution.SandboxID != ""
}

func TestExecutorSandboxExemptionRequiresActualShellAttestation(t *testing.T) {
	cases := []struct {
		name, sandbox string
		attest        bool
		allowed       bool
	}{
		{"local", "", false, false}, {"unattested id", "remote-id", false, false},
		{"snapshot", "snapshot:copy", true, false}, {"actual sandbox", "remote-id", true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			called := false
			base := fakeTool{name: "shell", permission: tooldef.PermissionExecute, inspect: func(context.Context) { called = true }}
			var shell tooldef.Tool = base
			if c.attest {
				shell = attestedShell{base}
			}
			executor := Executor{Registry: fakeRegistry{tools: map[string]tooldef.Tool{"shell": shell}}}
			result, err := executor.Execute(context.Background(), generationcommand.ToolExecution{
				Session: &session.Session{ID: "demo", PermissionMode: session.PermissionModeAsk, WorkspaceSandboxID: c.sandbox},
				Calls:   []domainmessage.ToolCall{{ID: "call", Name: "shell", Arguments: `{"command":"echo sample"}`}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if called != c.allowed {
				t.Fatalf("execution=%v want=%v", called, c.allowed)
			}
			if !c.allowed && (len(result.Entries) != 2 || result.Entries[1].Status != domaintool.ResultStatusDenied) {
				t.Fatal("unisolated call did not produce denial audit entry")
			}
		})
	}
}

func TestExecutorSandboxDoesNotExemptOtherExecuteTools(t *testing.T) {
	called := false
	executor := Executor{Registry: fakeRegistry{tools: map[string]tooldef.Tool{
		"shell":    attestedShell{fakeTool{name: "shell", permission: tooldef.PermissionExecute}},
		"mcp_demo": fakeTool{name: "mcp_demo", permission: tooldef.PermissionExecute, inspect: func(context.Context) { called = true }},
	}}}
	_, err := executor.Execute(context.Background(), generationcommand.ToolExecution{
		Session: &session.Session{ID: "demo", PermissionMode: session.PermissionModeAsk, WorkspaceSandboxID: "remote-id"},
		Calls:   []domainmessage.ToolCall{{Name: "mcp_demo", Arguments: `{"command":"pwd"}`}},
	})
	if err != nil || called {
		t.Fatalf("non-shell call acquired sandbox approval: %v", err)
	}
}

func TestExecutorUsesTurnRegistryForSandboxAttestation(t *testing.T) {
	executor := Executor{Registry: fakeRegistry{tools: map[string]tooldef.Tool{"shell": attestedShell{fakeTool{name: "shell", permission: tooldef.PermissionExecute}}}}}
	ctx := toolruntime.WithExecution(context.Background(), toolruntime.Execution{SandboxID: "remote-id"})
	ctx = toolapplicationruntime.WithRegistry(ctx, fakeRegistry{tools: map[string]tooldef.Tool{"shell": fakeTool{name: "shell", permission: tooldef.PermissionExecute}}})
	if executor.sandboxedShell(ctx, "remote-id") {
		t.Fatal("attestation came from a different registry than execution")
	}
}

func TestExecutorKeepsEmptyCapabilityCeilingAndPublicationRefusal(t *testing.T) {
	for _, name := range []string{"read_file", "share_file"} {
		called, asked := false, false
		permission := tooldef.PermissionRead
		if name == "share_file" {
			permission = tooldef.PermissionExecute
		}
		executor := Executor{Registry: fakeRegistry{tools: map[string]tooldef.Tool{name: fakeTool{name: name, permission: permission, inspect: func(context.Context) { called = true }}}}}
		command := generationcommand.ToolExecution{
			Session: &session.Session{ID: "demo", PermissionMode: session.PermissionModeFull, PermissionCeiling: []tooldef.Permission{}},
			Calls:   []domainmessage.ToolCall{{Name: name}},
			Stream:  modelport.ChatStreamHandler{OnToolAsk: func(modelport.ToolPermissionRequest) bool { asked = true; return false }},
		}
		if _, err := executor.Execute(context.Background(), command); err != nil || called || asked {
			t.Fatalf("deny-all ceiling lost: %v called=%v asked=%v", err, called, asked)
		}
		if name == "share_file" {
			command.Session.PermissionCeiling = nil
			if _, err := executor.Execute(context.Background(), command); err != nil || called || !asked {
				t.Fatalf("full-mode publication ignored refusal: %v called=%v asked=%v", err, called, asked)
			}
		}
	}
}
