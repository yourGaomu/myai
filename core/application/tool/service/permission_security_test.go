package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	toolcommand "myai/core/application/tool/command"
	modelport "myai/core/port/model"
	"myai/core/session"
	tooldef "myai/core/tool/tool"
)

func TestShellAutoApprovalRequiresAllowedArguments(t *testing.T) {
	allowed := []string{
		"pwd", "date", "Get-Date", "ls -la", "go version", "node --version",
		"git --no-pager status --short", "git --no-pager branch --show-current",
		"git --no-pager diff --no-ext-diff --no-textconv --stat",
	}
	denied := []string{
		"git status", "git --no-pager branch demo", "git --no-pager status --help",
		"git --no-pager diff --stat", "git --no-pager log --max-count=101",
		"date --file=notes.txt", "Get-Date -Format o", "pwd extra", "node --version extra",
		"ls notes.txt", "ls -unknown", "pwd && whoami", "pwd | whoami", "pwd\nwhoami",
		"ls 'docs'", "ls docs/*", "echo sample",
	}
	for _, command := range allowed {
		if !isSafeReadOnlyCommand(command) {
			t.Errorf("safe command needs approval: %q", command)
		}
	}
	for _, command := range denied {
		if isSafeReadOnlyCommand(command) {
			t.Errorf("unapproved command or arguments accepted: %q", command)
		}
	}
}

func TestUnknownPermissionsAlwaysDenied(t *testing.T) {
	for _, permission := range []tooldef.Permission{"", "READ", "read-write", "custom"} {
		for _, mode := range []session.PermissionMode{session.PermissionModeReadonly, session.PermissionModeReadWrite, session.PermissionModeExecute, session.PermissionModeAsk, session.PermissionModeFull} {
			asked := false
			decision := (PermissionService{}).Allow(toolcommand.Permission{
				Name: "extension", Permission: permission, Mode: mode,
				Ask: func(toolcommand.PermissionRequest) bool { asked = true; return true },
			})
			if decision.Allowed || asked {
				t.Errorf("unknown permission %q in mode %q was not hard denied", permission, mode)
			}
		}
	}
}

func TestProtectedPathsAndPublicationRequireConsentInAllModes(t *testing.T) {
	workspace := t.TempDir()
	cases := []struct {
		name, path string
		permission tooldef.Permission
	}{
		{"write_file", ".git/HEAD", tooldef.PermissionWrite},
		{"edit_file", ".myai/state.json", tooldef.PermissionWrite},
		{"write_file", "skills/demo/SKILL.md", tooldef.PermissionWrite},
		{"write_file", "plugins/demo/plugin.json", tooldef.PermissionWrite},
		{"read_file", "config/demo.yaml", tooldef.PermissionRead},
		{"read_file", ".env.test", tooldef.PermissionRead},
		{"write_file", "../outside.txt", tooldef.PermissionWrite},
		{"share_file", "docs/report.txt", tooldef.PermissionExecute},
		{"install_skill", "docs/demo", tooldef.PermissionWrite},
	}
	for _, c := range cases {
		for _, mode := range []session.PermissionMode{session.PermissionModeAsk, session.PermissionModeFull} {
			raw, _ := json.Marshal(map[string]string{"path": c.path})
			cmd := toolcommand.Permission{Name: c.name, Permission: c.permission, Arguments: string(raw), WorkspaceRoot: workspace, Mode: mode}
			if decision := (PermissionService{}).Allow(cmd); decision.Allowed {
				t.Fatalf("%s %q bypassed consent in %s", c.name, c.path, mode)
			}
			asked := false
			cmd.Ask = func(request toolcommand.PermissionRequest) bool {
				asked = true
				if !request.RequireFreshConsent {
					t.Errorf("%s %q must request per-call consent", c.name, c.path)
				}
				return false
			}
			if decision := (PermissionService{}).Allow(cmd); decision.Allowed || !asked {
				t.Fatalf("%s %q ignored refusal", c.name, c.path)
			}
			cmd.Ask = func(toolcommand.PermissionRequest) bool { return true }
			if decision := (PermissionService{}).Allow(cmd); !decision.Allowed {
				t.Fatalf("explicit consent was ignored: %s", decision.Message)
			}
		}
	}
	cmd := toolcommand.Permission{Name: "write_file", Permission: tooldef.PermissionWrite, Arguments: `{"path":"src/demo.go"}`, WorkspaceRoot: workspace, Mode: session.PermissionModeAsk}
	if decision := (PermissionService{}).Allow(cmd); !decision.Allowed {
		t.Fatalf("ordinary workspace write should remain allowed: %s", decision.Message)
	}
}

func TestIsolatedExecuteExemptionIsShellOnly(t *testing.T) {
	for _, name := range []string{"mcp_demo_run", "share_file", "spawn_agent", "extension"} {
		if decision := assessAskApproval(toolcommand.Permission{Name: name, Permission: tooldef.PermissionExecute, Arguments: `{"command":"pwd"}`, Isolated: true}); decision != askApprovalAsk {
			t.Errorf("isolated %s acquired shell exemption", name)
		}
	}
}

func TestWorkspaceAutoApprovalRejectsSymlinkAliasAndEscape(t *testing.T) {
	workspace, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace, "reports")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if pathInsideWorkspace(workspace, "reports/new.txt") {
		t.Fatal("outside symlink acquired workspace exemption")
	}
	if err := os.Mkdir(filepath.Join(workspace, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(workspace, "config"), filepath.Join(workspace, "src")); err != nil {
		t.Fatal(err)
	}
	if pathInsideWorkspace(workspace, "src/new.txt") {
		t.Fatal("protected alias acquired workspace exemption")
	}
}

type securityCatalog struct{ permissions map[string]tooldef.Permission }

func (c securityCatalog) LLMToolsByPermission(allow func(tooldef.Permission) bool) []modelport.Tool {
	var result []modelport.Tool
	for name, permission := range c.permissions {
		if allow(permission) {
			result = append(result, modelport.Tool{Function: &modelport.FunctionDefinition{Name: name}})
		}
	}
	return result
}

func TestSelectionHonorsCapabilityCeilingAndRejectsUnknown(t *testing.T) {
	catalog := securityCatalog{permissions: map[string]tooldef.Permission{"read_file": tooldef.PermissionRead, "write_file": tooldef.PermissionWrite, "shell": tooldef.PermissionExecute, "extension": ""}}
	service := SelectionService{Catalog: catalog}
	for _, mode := range []session.PermissionMode{session.PermissionModeFull, session.PermissionModeAsk} {
		current := &session.Session{PermissionMode: mode, PermissionCeiling: []tooldef.Permission{tooldef.PermissionRead, tooldef.PermissionWrite}}
		tools := service.ToolsForSession(current, false)
		if len(tools) != 2 {
			t.Fatalf("ceiling selection = %#v", tools)
		}
		for _, tool := range tools {
			if tool.Function.Name == "shell" || tool.Function.Name == "extension" {
				t.Fatal("tool above ceiling was exposed")
			}
		}
		current.PermissionCeiling = []tooldef.Permission{}
		if len(service.ToolsForSession(current, false)) != 0 {
			t.Fatal("empty deny-all ceiling exposed tools")
		}
	}
}
