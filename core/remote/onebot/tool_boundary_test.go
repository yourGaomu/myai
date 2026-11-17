package onebot

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	toolcommand "myai/core/application/tool/command"
	toolservice "myai/core/application/tool/service"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	modelport "myai/core/port/model"
	"myai/core/session"
	tooldef "myai/core/tool/tool"
)

type boundaryTool struct {
	name   string
	called *int
}

func (t boundaryTool) Name() string                   { return t.name }
func (t boundaryTool) Description() string            { return t.name }
func (t boundaryTool) Permission() tooldef.Permission { return tooldef.PermissionRead }
func (t boundaryTool) Schema() any                    { return nil }
func (t boundaryTool) Call(context.Context, json.RawMessage) (tooldef.ToolOutput, error) {
	*t.called++
	return tooldef.SuccessOutput("fixture"), nil
}

type boundaryRegistry map[string]tooldef.Tool

func (r boundaryRegistry) GetTool(name string) (tooldef.Tool, error) {
	if tool := r[name]; tool != nil {
		return tool, nil
	}
	return nil, errors.New("tool not found")
}
func (r boundaryRegistry) LLMToolsByPermission(allow func(tooldef.Permission) bool) []modelport.Tool {
	var result []modelport.Tool
	for name, tool := range r {
		if allow(tool.Permission()) {
			result = append(result, modelport.Tool{Function: &modelport.FunctionDefinition{Name: name}})
		}
	}
	return result
}

func TestOneBotAllowlistBlocksDiscoveryAndForgedWorkspaceCalls(t *testing.T) {
	for _, role := range []Role{RoleUser, RoleAdmin} {
		chat := newMockChatFacade()
		bot, err := NewBot(Config{}, nil, chat)
		if err != nil {
			t.Fatal(err)
		}
		if err := bot.configureSessionAccess(context.Background(), "session", Event{MessageType: "group"}, role); err != nil {
			t.Fatal(err)
		}
		called, managed := 0, 0
		registry := boundaryRegistry{}
		for _, name := range []string{"read_file", "read_asset", "list_files", "search_files", "memory_search", "knowledge_search", "mcp_demo", "list_mcp_resources"} {
			registry[name] = boundaryTool{name: name, called: &called}
		}
		registry["onebot_get_status"] = boundaryTool{name: "onebot_get_status", called: &managed}
		current := &session.Session{PermissionMode: session.PermissionModeReadonly, AllowedTools: chat.allowedTools["session"], EnforceToolAllowlist: chat.enforcedTools["session"], RAGSettings: chat.ragSettings["session"]}
		tools := (toolservice.SelectionService{Catalog: registry}).ToolsForSession(current, false)
		for _, tool := range tools {
			if tool.Function.Name != "onebot_get_status" {
				t.Errorf("workspace tool was exposed to %s: %s", role, tool.Function.Name)
			}
		}
		var calls []domainmessage.ToolCall
		for name := range registry {
			calls = append(calls, domainmessage.ToolCall{ID: name, Name: name})
		}
		result, err := (toolservice.ExecutionService{Registry: registry}).Execute(context.Background(), toolcommand.Execution{PermissionMode: current.PermissionMode, AllowedTools: current.AllowedTools, EnforceToolAllowlist: current.EnforceToolAllowlist, Calls: calls})
		if err != nil {
			t.Fatal(err)
		}
		if called != 0 {
			t.Fatal("forged workspace call bypassed execution allowlist")
		}
		wantManaged := 0
		if role == RoleAdmin {
			wantManaged = 1
		}
		if managed != wantManaged {
			t.Fatalf("management tool permission = %d want %d", managed, wantManaged)
		}
		for _, entry := range result.Entries {
			if entry.Kind == domaintool.ExecutionEntryToolResult && entry.ToolName != "onebot_get_status" && entry.Status != domaintool.ResultStatusDenied {
				t.Errorf("workspace tool %s did not record a denial", entry.ToolName)
			}
		}
	}
}
