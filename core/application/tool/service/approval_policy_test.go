package service

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"

	toolcommand "myai/core/application/tool/command"
	tooldef "myai/core/tool/tool"
)

func TestAssessAskApprovalAutoApprovesWorkspaceWrite(t *testing.T) {
	workspace := t.TempDir()
	decision := assessAskApproval(toolcommand.Permission{
		Name:          "write_file",
		Arguments:     `{"path":"README.md"}`,
		Permission:    tooldef.PermissionWrite,
		WorkspaceRoot: workspace,
	})
	if decision != askApprovalAuto {
		t.Fatalf("expected workspace write to auto-approve, got %v", decision)
	}
}

func TestAssessAskApprovalAsksWriteOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(filepath.Dir(workspace), "outside.txt")
	payload, err := json.Marshal(map[string]string{"path": outside})
	if err != nil {
		t.Fatal(err)
	}
	decision := assessAskApproval(toolcommand.Permission{
		Name:          "write_file",
		Arguments:     string(payload),
		Permission:    tooldef.PermissionWrite,
		WorkspaceRoot: workspace,
	})
	if decision != askApprovalAsk {
		t.Fatalf("expected outside write to ask, got %v", decision)
	}
}

func TestAssessAskApprovalAutoApprovesSafeShell(t *testing.T) {
	for _, command := range []string{
		`{"command":"Get-Date"}`,
		`{"command":"date"}`,
		`{"command":"git --no-pager status --short"}`,
		`{"command":"git --no-pager log --oneline --max-count=5"}`,
	} {
		decision := assessAskApproval(toolcommand.Permission{
			Name:       "shell",
			Arguments:  command,
			Permission: tooldef.PermissionExecute,
		})
		if decision != askApprovalAuto {
			t.Fatalf("expected safe shell %s to auto-approve, got %v", command, decision)
		}
	}
}

func TestAssessAskApprovalAsksUnlistedShell(t *testing.T) {
	decision := assessAskApproval(toolcommand.Permission{
		Name:       "shell",
		Arguments:  `{"command":"echo sample"}`,
		Permission: tooldef.PermissionExecute,
	})
	if decision != askApprovalAsk {
		t.Fatalf("expected dangerous shell to ask, got %v", decision)
	}
}

func TestAssessAskApprovalAutoApprovesIsolatedNonDangerousShell(t *testing.T) {
	decision := assessAskApproval(toolcommand.Permission{
		Name:       "shell",
		Arguments:  `{"command":"python main.py"}`,
		Permission: tooldef.PermissionExecute,
		Isolated:   true,
	})
	if decision != askApprovalAuto {
		t.Fatalf("expected isolated non-dangerous shell to auto-approve, got %v", decision)
	}
}

func TestAssessAskApprovalAsksUnparsedIsolatedShell(t *testing.T) {
	decision := assessAskApproval(toolcommand.Permission{
		Name:       "shell",
		Arguments:  `{"command":"pwd | whoami"}`,
		Permission: tooldef.PermissionExecute,
		Isolated:   true,
	})
	if decision != askApprovalAsk {
		t.Fatalf("expected isolated dangerous shell to ask, got %v", decision)
	}
}

func TestPathInsideWorkspaceRejectsParentEscape(t *testing.T) {
	workspace := t.TempDir()
	if pathInsideWorkspace(workspace, filepath.Join("..", "secret.txt")) {
		t.Fatal("parent escape must not count as inside workspace")
	}
	if runtime.GOOS == "windows" && pathInsideWorkspace(`D:\workspace`, `C:\Windows\System32`) {
		t.Fatal("other drive must not count as inside workspace")
	}
}

func TestGenerateImageApprovalCoversAllPathInputs(t *testing.T) {
	workspace := t.TempDir()
	cases := []struct {
		name, args string
		want       askApproval
	}{
		{"default output", `{"prompt":"scene"}`, askApprovalAuto},
		{"workspace reference", `{"reference_images":["images/ref.png"]}`, askApprovalAuto},
		{"workspace output", `{"output_path":"generated/scene.png"}`, askApprovalAuto},
		{"protected output", `{"output_path":".env"}`, askApprovalAsk},
		{"outside reference", `{"reference_images":["../outside.png"]}`, askApprovalAsk},
		{"protected mask", `{"mask_image":"config/secret.png"}`, askApprovalAsk},
		{"malformed reference type", `{"reference_images":"images/ref.png"}`, askApprovalAsk},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := assessAskApproval(toolcommand.Permission{Name: "generate_image", Arguments: tc.args, Permission: tooldef.PermissionWrite, WorkspaceRoot: workspace})
			if got != tc.want {
				t.Fatalf("approval=%v want=%v", got, tc.want)
			}
		})
	}
}
