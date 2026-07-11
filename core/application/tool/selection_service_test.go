package toolapp

import (
	"testing"

	modelport "myai/core/port/model"
	"myai/core/session"
	tooldef "myai/core/tool/tool"
)

func TestSelectionServiceFiltersReadonlyPermissionMode(t *testing.T) {
	catalog := recordingCatalog{
		permissions: []tooldef.Permission{
			tooldef.PermissionRead,
			tooldef.PermissionWrite,
			tooldef.PermissionExecute,
		},
	}

	tools := SelectionService{Catalog: catalog}.ToolsForSession(&session.Session{
		PermissionMode: session.PermissionModeReadonly,
		AgentMode:      session.AgentModeChat,
	}, false)

	if len(tools) != 1 || tools[0].Function.Name != "read" {
		t.Fatalf("expected readonly mode to expose only read tool, got %#v", tools)
	}
}

func TestSelectionServiceUsesModePolicyForPlanMode(t *testing.T) {
	catalog := recordingCatalog{
		permissions: []tooldef.Permission{
			tooldef.PermissionRead,
			tooldef.PermissionWrite,
		},
	}

	tools := SelectionService{
		Catalog: catalog,
		ModePolicy: &recordingModePolicy{
			denyWritesInPlan: true,
		},
	}.ToolsForSession(&session.Session{
		PermissionMode: session.PermissionModeFull,
		AgentMode:      session.AgentModePlan,
	}, false)

	if len(tools) != 1 || tools[0].Function.Name != "read" {
		t.Fatalf("expected plan mode policy to hide write tool, got %#v", tools)
	}
}

func TestSelectionServicePassesForceChatModeToModePolicy(t *testing.T) {
	catalog := recordingCatalog{
		permissions: []tooldef.Permission{
			tooldef.PermissionWrite,
		},
	}
	policy := &recordingModePolicy{}

	tools := SelectionService{
		Catalog:    catalog,
		ModePolicy: policy,
	}.ToolsForSession(&session.Session{
		PermissionMode: session.PermissionModeFull,
		AgentMode:      session.AgentModePlan,
	}, true)

	if len(tools) != 1 || tools[0].Function.Name != "write" {
		t.Fatalf("expected forced chat mode to keep write tool available, got %#v", tools)
	}
	if !policy.forceChatMode {
		t.Fatal("expected forceChatMode to be passed to mode policy")
	}
}

type recordingCatalog struct {
	permissions []tooldef.Permission
}

func (c recordingCatalog) LLMToolsByPermission(allow func(tooldef.Permission) bool) []modelport.Tool {
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

type recordingModePolicy struct {
	denyWritesInPlan bool
	forceChatMode    bool
}

func (p *recordingModePolicy) AllowsToolPermission(permission tooldef.Permission, agentMode session.AgentMode, forceChatMode bool) bool {
	p.forceChatMode = forceChatMode
	if forceChatMode {
		return true
	}
	if p.denyWritesInPlan && session.NormalizeAgentMode(agentMode) == session.AgentModePlan && tooldef.NormalizePermission(permission) != tooldef.PermissionRead {
		return false
	}
	return true
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
