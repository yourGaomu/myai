package service

import (
	"context"
	"encoding/json"
	"testing"

	toolcommand "myai/core/application/tool/command"
	domainmessage "myai/core/domain/message"
	"myai/core/session"
	"myai/core/tool"
	tooldef "myai/core/tool/tool"
)

type publicationTool struct{ calls *int }

func (publicationTool) Name() string                   { return "extension_publish" }
func (publicationTool) Description() string            { return "Publish a file" }
func (publicationTool) Schema() any                    { return map[string]any{"type": "object"} }
func (publicationTool) Permission() tooldef.Permission { return tooldef.PermissionExecute }
func (publicationTool) RequiresExplicitApproval() bool { return true }
func (t publicationTool) Call(context.Context, json.RawMessage) (tooldef.ToolOutput, error) {
	*t.calls++
	return tooldef.SuccessOutput("published"), nil
}

func TestExplicitPublicationMetadataRequiresConsentInFullMode(t *testing.T) {
	command := toolcommand.Permission{
		Name: "extension_publish", Permission: tooldef.PermissionExecute,
		Mode: session.PermissionModeFull, ExplicitApproval: true,
	}
	if (PermissionService{}).Allow(command).Allowed {
		t.Fatal("publication metadata bypassed mandatory consent")
	}
	asked := 0
	command.Ask = func(request toolcommand.PermissionRequest) bool {
		asked++
		if !request.RequireFreshConsent {
			t.Error("explicit publication metadata must request per-call consent")
		}
		return true
	}
	if !(PermissionService{}).Allow(command).Allowed || asked != 1 {
		t.Fatal("publication did not use explicit consent")
	}
	command.Mode = session.PermissionModeReadonly
	if (PermissionService{}).Allow(command).Allowed || asked != 1 {
		t.Fatal("consent elevated a read-only capability")
	}
}

func TestExecutionPropagatesPublicationMetadataBeforeCallingTool(t *testing.T) {
	calls := 0
	registry := tool.NewRegisterTools()
	registry.Register(publicationTool{calls: &calls})
	command := toolcommand.Execution{
		SessionID: "session", PermissionMode: session.PermissionModeFull,
		Calls: []domainmessage.ToolCall{{ID: "call", Name: "extension_publish", Arguments: `{}`}},
	}
	executor := ExecutionService{Registry: registry}
	if _, err := executor.Execute(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("publication ran without an approval handler")
	}
	asked := 0
	command.Callbacks.OnToolAsk = func(toolcommand.PermissionRequest) bool { asked++; return false }
	if _, err := executor.Execute(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if calls != 0 || asked != 1 {
		t.Fatal("publication ignored refusal")
	}
	command.Callbacks.OnToolAsk = func(toolcommand.PermissionRequest) bool { asked++; return true }
	if _, err := executor.Execute(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || asked != 2 {
		t.Fatal("approved publication did not execute exactly once")
	}
}

var _ tooldef.Tool = publicationTool{}
var _ tooldef.ExplicitApproval = publicationTool{}
