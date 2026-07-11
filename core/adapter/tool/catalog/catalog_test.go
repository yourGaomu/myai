package catalog

import (
	"testing"

	modelport "myai/core/port/model"
	"myai/core/session"
	tooldef "myai/core/tool/tool"
)

func TestCatalogUsesDefaultModePolicy(t *testing.T) {
	catalog := Catalog{
		Tools: fakeCatalog{
			permissions: []tooldef.Permission{
				tooldef.PermissionRead,
				tooldef.PermissionWrite,
			},
		},
	}

	tools := catalog.ToolsForSession(&session.Session{
		PermissionMode: session.PermissionModeFull,
		AgentMode:      session.AgentModePlan,
	}, false)

	if len(tools) != 1 || tools[0].Function.Name != "read" {
		t.Fatalf("expected default plan mode policy to expose only read tool, got %#v", tools)
	}
}

func TestCatalogAllowsForcedChatMode(t *testing.T) {
	catalog := Catalog{
		Tools: fakeCatalog{
			permissions: []tooldef.Permission{
				tooldef.PermissionWrite,
			},
		},
	}

	tools := catalog.ToolsForSession(&session.Session{
		PermissionMode: session.PermissionModeFull,
		AgentMode:      session.AgentModePlan,
	}, true)

	if len(tools) != 1 || tools[0].Function.Name != "write" {
		t.Fatalf("expected forced chat mode to expose write tool, got %#v", tools)
	}
}

type fakeCatalog struct {
	permissions []tooldef.Permission
}

func (c fakeCatalog) LLMToolsByPermission(allow func(tooldef.Permission) bool) []modelport.Tool {
	tools := make([]modelport.Tool, 0, len(c.permissions))
	for _, permission := range c.permissions {
		if allow != nil && !allow(permission) {
			continue
		}
		tools = append(tools, modelport.Tool{
			Type: "function",
			Function: &modelport.FunctionDefinition{
				Name: permissionName(permission),
			},
		})
	}
	return tools
}

func permissionName(permission tooldef.Permission) string {
	switch tooldef.NormalizePermission(permission) {
	case tooldef.PermissionWrite:
		return "write"
	case tooldef.PermissionExecute:
		return "execute"
	default:
		return "read"
	}
}
