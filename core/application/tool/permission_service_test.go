package toolapp

import (
	"strings"
	"testing"

	"myai/core/session"
	tooldef "myai/core/tool/tool"
)

func TestPermissionServiceAllowsRead(t *testing.T) {
	decision := PermissionService{}.Allow(PermissionCommand{
		Name:       "read_file",
		Permission: tooldef.PermissionRead,
		Mode:       session.PermissionModeReadonly,
	})
	if !decision.Allowed {
		t.Fatalf("expected read permission to be allowed: %#v", decision)
	}
}

func TestPermissionServiceDeniesWriteInReadonlyMode(t *testing.T) {
	decision := PermissionService{}.Allow(PermissionCommand{
		Name:       "write_file",
		Permission: tooldef.PermissionWrite,
		Mode:       session.PermissionModeReadonly,
	})
	if decision.Allowed || !strings.Contains(decision.Message, "readonly") {
		t.Fatalf("expected readonly denial: %#v", decision)
	}
}

func TestPermissionServiceAsksInAskMode(t *testing.T) {
	var request PermissionRequest
	decision := PermissionService{}.Allow(PermissionCommand{
		Name:       "write_file",
		Arguments:  `{"path":"a.txt"}`,
		Permission: tooldef.PermissionWrite,
		Mode:       session.PermissionModeAsk,
		Ask: func(r PermissionRequest) bool {
			request = r
			return true
		},
	})
	if !decision.Allowed {
		t.Fatalf("expected ask approval: %#v", decision)
	}
	if request.Name != "write_file" || request.Permission != tooldef.PermissionWrite || request.Mode != session.PermissionModeAsk {
		t.Fatalf("unexpected permission request: %#v", request)
	}
}

func TestPermissionServiceAutoApprovesWorkspaceWriteInAskMode(t *testing.T) {
	asked := false
	decision := PermissionService{}.Allow(PermissionCommand{
		Name:          "write_file",
		Arguments:     `{"path":"a.txt"}`,
		Permission:    tooldef.PermissionWrite,
		Mode:          session.PermissionModeAsk,
		WorkspaceRoot: t.TempDir(),
		Ask: func(PermissionRequest) bool {
			asked = true
			return false
		},
	})
	if !decision.Allowed || asked {
		t.Fatalf("expected workspace write to auto-approve without asking: %#v", decision)
	}
}

func TestPermissionServiceAutoApprovesSafeShellInAskMode(t *testing.T) {
	asked := false
	decision := PermissionService{}.Allow(PermissionCommand{
		Name:       "shell",
		Arguments:  `{"command":"Get-Date"}`,
		Permission: tooldef.PermissionExecute,
		Mode:       session.PermissionModeAsk,
		Ask: func(PermissionRequest) bool {
			asked = true
			return false
		},
	})
	if !decision.Allowed || asked {
		t.Fatalf("expected safe shell to auto-approve without asking: %#v", decision)
	}
}

func TestPermissionServiceKeepsReadonlyAsHardBoundary(t *testing.T) {
	decision := PermissionService{}.Allow(PermissionCommand{
		Name:       "shell",
		Permission: tooldef.PermissionExecute,
		Mode:       session.PermissionModeReadonly,
	})
	if decision.Allowed || !strings.Contains(decision.Message, "readonly") {
		t.Fatalf("expected readonly mode to remain a hard boundary: %#v", decision)
	}
}
