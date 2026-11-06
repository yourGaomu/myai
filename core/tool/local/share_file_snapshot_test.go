package local

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	toolcommand "myai/core/application/tool/command"
	toolservice "myai/core/application/tool/service"
	"myai/core/session"
)

func TestSnapshotArtifactStillRequiresPublicationConsentAndCanUpload(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), ".myai", "subagent-snapshots", "job", "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "report.txt"), []byte("public fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	uploader := &fakeAssetUploader{}
	tool := NewShareFileToolWithWorkspaceAndUploader(workspace, uploader)
	for _, path := range []string{"report.txt", filepath.Join(workspace, "report.txt")} {
		raw, _ := json.Marshal(map[string]string{"path": path})
		command := toolcommand.Permission{Name: tool.Name(), Arguments: string(raw), Permission: tool.Permission(), Mode: session.PermissionModeFull, WorkspaceRoot: workspace}
		if decision := (toolservice.PermissionService{}).Allow(command); decision.Allowed {
			t.Fatal("child artifact publication bypassed consent")
		}
		command.Ask = func(toolcommand.PermissionRequest) bool { return true }
		if decision := (toolservice.PermissionService{}).Allow(command); !decision.Allowed {
			t.Fatalf("explicit publication consent rejected: %s", decision.Message)
		}
		if _, err := tool.Call(context.Background(), raw); err != nil {
			t.Fatalf("approved ordinary snapshot artifact could not upload: %v", err)
		}
	}
	if uploader.content != "public fixture" {
		t.Fatal("approved artifact was not uploaded")
	}
	if err := os.WriteFile(filepath.Join(workspace, ".env.test"), []byte("private fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	uploader.request.Reader = nil
	if _, err := tool.Call(context.Background(), []byte(`{"path":".env.test"}`)); err == nil {
		t.Fatal("child sensitive content was shared")
	}
	if uploader.request.Reader != nil {
		t.Fatal("child sensitive content reached uploader")
	}
}
