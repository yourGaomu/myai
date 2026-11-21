package local

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

func TestGetEnvironmentUsesCurrentTimeAndActiveWorkspace(t *testing.T) {
	tool := NewGetEnvironmentToolWithWorkspace("default-workspace")
	now := time.Date(2026, 11, 21, 15, 4, 5, 0, time.FixedZone("NPT", 5*3600+45*60))
	tool.now = func() time.Time { return now }
	ctx := toolruntime.WithExecution(context.Background(), toolruntime.Execution{WorkspaceRoot: "active-workspace"})
	if tool.Permission() != tooldef.PermissionRead {
		t.Fatal("environment query must be read-only")
	}
	for _, want := range []string{"2026-11-21T15:04:05+05:45", "2026-11-21T15:04:06+05:45"} {
		output, err := tool.Call(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		var facts map[string]string
		if err := json.Unmarshal([]byte(output.Content), &facts); err != nil {
			t.Fatal(err)
		}
		if facts["local_time"] != want || facts["timezone"] != "NPT" || facts["workspace_root"] != "active-workspace" {
			t.Fatalf("environment facts: %v", facts)
		}
		now = now.Add(time.Second)
	}
	isolated := toolruntime.WithExecution(context.Background(), toolruntime.Execution{SandboxID: "sandbox"})
	output, err := tool.Call(isolated, nil)
	if err != nil {
		t.Fatal(err)
	}
	var facts map[string]string
	if err := json.Unmarshal([]byte(output.Content), &facts); err != nil {
		t.Fatal(err)
	}
	if _, exists := facts["workspace_root"]; exists {
		t.Fatalf("sandbox received the host workspace: %v", facts)
	}
}
