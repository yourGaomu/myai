package local

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	workspaceopensandbox "myai/core/adapter/workspace/opensandbox"
	workspacerouter "myai/core/adapter/workspace/router"
	toolruntime "myai/core/tool/runtimecontext"
)

func TestShareFileRefusesProtectedPathsBeforeUploading(t *testing.T) {
	for _, path := range []string{".env.staging", "application.yaml", "relay.yaml", "config/demo.txt", "privatekeys/demo", "keys/demo.pem", ".git/HEAD", ".myai/demo.txt", "skills/demo/SKILL.md", "plugins/demo/plugin.json"} {
		t.Run(path, func(t *testing.T) {
			workspace := t.TempDir()
			target := filepath.Join(workspace, filepath.FromSlash(path))
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte("test fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
			uploader := &fakeAssetUploader{}
			raw, _ := json.Marshal(map[string]string{"path": path})
			if _, err := NewShareFileToolWithWorkspaceAndUploader(workspace, uploader).Call(context.Background(), raw); err == nil {
				t.Fatal("protected file was shared")
			}
			if uploader.request.Reader != nil {
				t.Fatal("uploader received a protected file")
			}
		})
	}
}

func TestShareFileRefusesSymlinkTargetAndProtectedAlias(t *testing.T) {
	workspace, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "demo.txt"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "demo.txt"), filepath.Join(workspace, "report.txt")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	uploader := &fakeAssetUploader{}
	tool := NewShareFileToolWithWorkspaceAndUploader(workspace, uploader)
	if _, err := tool.Call(context.Background(), []byte(`{"path":"report.txt"}`)); err == nil {
		t.Fatal("outside symlink was shared")
	}
	if err := os.Mkdir(filepath.Join(workspace, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "config", "demo.txt"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(workspace, "config", "demo.txt"), filepath.Join(workspace, "public.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Call(context.Background(), []byte(`{"path":"public.txt"}`)); err == nil {
		t.Fatal("sensitive symlink target was shared")
	}
	if err := os.WriteFile(filepath.Join(workspace, "ordinary.txt"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(workspace, "ordinary.txt"), filepath.Join(workspace, ".env.demo")); err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Call(context.Background(), []byte(`{"path":".env.demo"}`)); err == nil {
		t.Fatal("protected original path was shared")
	}
	if uploader.request.Reader != nil {
		t.Fatal("uploader was called for a denied alias")
	}
}

func TestShellSandboxAttestationRequiresConcreteRemoteRoute(t *testing.T) {
	remote := &workspaceopensandbox.Manager{}
	routed := &workspacerouter.Manager{Remote: remote}
	for _, sandboxID := range []string{"", "snapshot:copy", "SNAPSHOT:copy"} {
		ctx := toolruntime.WithExecution(context.Background(), toolruntime.Execution{SandboxID: sandboxID})
		if NewShellToolWithWorkspaceAndRemote(".", nil, routed).IsSandboxedShell(ctx) {
			t.Fatalf("%q asserted sandbox isolation", sandboxID)
		}
	}
	ctx := toolruntime.WithExecution(context.Background(), toolruntime.Execution{SandboxID: "remote-sandbox"})
	if NewShellToolWithWorkspace(".", nil).IsSandboxedShell(ctx) {
		t.Fatal("local shell asserted sandbox isolation")
	}
	if NewShellToolWithWorkspaceAndRemote(".", nil, &workspacerouter.Manager{}).IsSandboxedShell(ctx) {
		t.Fatal("router without remote asserted isolation")
	}
	if !NewShellToolWithWorkspaceAndRemote(".", nil, routed).IsSandboxedShell(ctx) {
		t.Fatal("OpenSandbox route did not attest isolation")
	}
}
