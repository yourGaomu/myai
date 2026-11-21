package local

import (
	"context"
	"encoding/json"
	"time"

	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

// GetEnvironmentTool makes volatile facts available without adding them to every prompt.
type GetEnvironmentTool struct {
	workspace string
	now       func() time.Time
}

func NewGetEnvironmentToolWithWorkspace(workspace string) *GetEnvironmentTool {
	return &GetEnvironmentTool{workspace: workspace, now: time.Now}
}

func (*GetEnvironmentTool) Name() string { return "get_environment" }

func (*GetEnvironmentTool) Description() string {
	return "Get the current local time with its UTC offset and the active workspace root. Call only when these facts are needed."
}

func (*GetEnvironmentTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (*GetEnvironmentTool) Permission() tooldef.Permission { return tooldef.PermissionRead }

func (t *GetEnvironmentTool) Call(ctx context.Context, _ json.RawMessage) (tooldef.ToolOutput, error) {
	now := t.now()
	zone, _ := now.Zone()
	workspace := toolruntime.WorkspaceRoot(ctx, t.workspace)
	if execution, ok := toolruntime.CurrentExecution(ctx); ok && (execution.SandboxID != "" || execution.ParentSessionID != "") {
		// A child or sandbox without a workspace must not expose the host fallback.
		workspace = execution.WorkspaceRoot
	}
	data, err := json.Marshal(struct {
		LocalTime     string `json:"local_time"`
		Timezone      string `json:"timezone"`
		WorkspaceRoot string `json:"workspace_root,omitempty"`
	}{now.Format(time.RFC3339), zone, workspace})
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	return tooldef.SuccessOutput(string(data)), nil
}
