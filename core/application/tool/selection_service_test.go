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

func TestSelectionServiceKeepsMemorySearchIndependentOfRAGMode(t *testing.T) {
	catalog := namedCatalog{names: []string{"knowledge_search", "memory_search", "read_file"}}
	for _, test := range []struct {
		name string
		mode session.RetrievalMode
		want bool
	}{
		{name: "auto", mode: session.RetrievalModeAuto, want: true},
		{name: "off", mode: session.RetrievalModeOff},
		{name: "manual", mode: session.RetrievalModeManual},
		{name: "always", mode: session.RetrievalModeAlways},
	} {
		t.Run(test.name, func(t *testing.T) {
			tools := SelectionService{Catalog: catalog}.ToolsForSession(&session.Session{
				RAGSettings:    session.RAGSettings{Mode: test.mode},
				PermissionMode: session.PermissionModeFull,
			}, false)
			foundKnowledgeSearch := false
			foundMemorySearch := false
			for _, tool := range tools {
				if tool.Function != nil && tool.Function.Name == "knowledge_search" {
					foundKnowledgeSearch = true
				}
				if tool.Function != nil && tool.Function.Name == "memory_search" {
					foundMemorySearch = true
				}
			}
			if foundKnowledgeSearch != test.want {
				t.Fatalf("knowledge_search availability = %v, want %v; tools=%#v", foundKnowledgeSearch, test.want, tools)
			}
			if !foundMemorySearch {
				t.Fatalf("memory_search must be independent of RAG mode; tools=%#v", tools)
			}
		})
	}
}

func TestSelectionServiceFiltersReadWriteAndExecuteModes(t *testing.T) {
	catalog := recordingCatalog{permissions: []tooldef.Permission{
		tooldef.PermissionRead, tooldef.PermissionWrite, tooldef.PermissionExecute,
	}}
	readWrite := SelectionService{Catalog: catalog}.ToolsForSession(&session.Session{
		PermissionMode: session.PermissionModeReadWrite,
	}, false)
	if len(readWrite) != 2 || readWrite[0].Function.Name != "read" || readWrite[1].Function.Name != "write" {
		t.Fatalf("unexpected read-write tools: %#v", readWrite)
	}
	execute := SelectionService{Catalog: catalog}.ToolsForSession(&session.Session{
		PermissionMode: session.PermissionModeExecute,
	}, false)
	if len(execute) != 2 || execute[0].Function.Name != "read" || execute[1].Function.Name != "execute" {
		t.Fatalf("unexpected execute tools: %#v", execute)
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

func TestSelectionServiceLeavesGlobalToolsAvailableForOrdinarySession(t *testing.T) {
	catalog := recordingCatalog{permissions: []tooldef.Permission{
		tooldef.PermissionRead,
		tooldef.PermissionWrite,
	}}

	tools := SelectionService{Catalog: catalog}.ToolsForSession(&session.Session{
		Kind:           session.KindUser,
		PermissionMode: session.PermissionModeFull,
		AllowedTools:   nil,
	}, false)

	if len(tools) != 2 {
		t.Fatalf("expected ordinary session to inherit all globally available tools, got %#v", tools)
	}
}

func TestSelectionServiceHidesAllToolsForEnforcedEmptyAllowlist(t *testing.T) {
	catalog := recordingCatalog{permissions: []tooldef.Permission{
		tooldef.PermissionRead,
		tooldef.PermissionWrite,
	}}

	tools := SelectionService{Catalog: catalog}.ToolsForSession(&session.Session{
		Kind:                 session.KindSubagent,
		PermissionMode:       session.PermissionModeFull,
		AllowedTools:         []string{},
		EnforceToolAllowlist: true,
	}, false)

	if len(tools) != 0 {
		t.Fatalf("expected enforced empty allowlist to expose no tools, got %#v", tools)
	}
}

func TestSelectionServiceFiltersToolsUsingEnforcedAllowlist(t *testing.T) {
	catalog := recordingCatalog{permissions: []tooldef.Permission{
		tooldef.PermissionRead,
		tooldef.PermissionWrite,
		tooldef.PermissionExecute,
	}}

	tools := SelectionService{Catalog: catalog}.ToolsForSession(&session.Session{
		Kind:                 session.KindSubagent,
		PermissionMode:       session.PermissionModeFull,
		AllowedTools:         []string{"write"},
		EnforceToolAllowlist: true,
	}, false)

	if len(tools) != 1 || tools[0].Function == nil || tools[0].Function.Name != "write" {
		t.Fatalf("expected allowlist to expose only write, got %#v", tools)
	}
}

type recordingCatalog struct {
	permissions []tooldef.Permission
}

type namedCatalog struct {
	names []string
}

func (c namedCatalog) LLMToolsByPermission(allow func(tooldef.Permission) bool) []modelport.Tool {
	tools := make([]modelport.Tool, 0, len(c.names))
	for _, name := range c.names {
		if allow != nil && !allow(tooldef.PermissionRead) {
			continue
		}
		tools = append(tools, modelport.Tool{
			Type:     "function",
			Function: &modelport.FunctionDefinition{Name: name},
		})
	}
	return tools
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
