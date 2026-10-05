package catalog

import (
	"context"
	"encoding/json"
	toolcommand "myai/core/application/tool/command"
	toolservice "myai/core/application/tool/service"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	"myai/core/session"
	"myai/core/tool"
	tooldef "myai/core/tool/tool"
	"testing"
)

type versionedTool string

func (t versionedTool) Name() string                   { return "version" }
func (t versionedTool) Description() string            { return string(t) }
func (t versionedTool) Schema() any                    { return map[string]any{"type": "object"} }
func (t versionedTool) Permission() tooldef.Permission { return tooldef.PermissionRead }
func (t versionedTool) Call(context.Context, json.RawMessage) (tooldef.ToolOutput, error) {
	return tooldef.SuccessOutput(string(t)), nil
}

func TestTurnSnapshotBindsDiscoveryAndExecution(t *testing.T) {
	registry := tool.NewRegisterTools()
	registry.RegisterSource("mcp:test", []tooldef.Tool{versionedTool("old")})
	catalog := Catalog{Tools: registry}
	ctx, oldCatalog, release := catalog.BeginTurn(context.Background())
	defer release()
	unlock := registry.BeginReload()
	registry.RegisterSource("mcp:test", []tooldef.Tool{versionedTool("new")})
	unlock()
	current := &session.Session{PermissionMode: session.PermissionModeReadonly}
	if oldCatalog.ToolsForSession(current, false)[0].Function.Description != "old" {
		t.Fatal("old turn discovery changed")
	}
	if catalog.ToolsForSession(current, false)[0].Function.Description != "new" {
		t.Fatal("new discovery did not update")
	}
	var result string
	_, err := (toolservice.ExecutionService{Registry: registry}).Execute(ctx, toolcommand.Execution{
		PermissionMode: session.PermissionModeReadonly,
		Calls:          []domainmessage.ToolCall{{ID: "call", Name: "version", Arguments: `{}`}},
		Callbacks:      toolcommand.ExecutionCallbacks{OnToolResult: func(_, _ string, output domaintool.ToolOutput) { result = output.Content }},
	})
	if err != nil || result != "old" {
		t.Fatalf("execution used different generation: %q %v", result, err)
	}
}
