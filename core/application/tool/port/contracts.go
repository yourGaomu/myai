package port

import (
	"context"
	"encoding/json"

	toolcommand "myai/core/application/tool/command"
	toolresult "myai/core/application/tool/result"
	domaintool "myai/core/domain/tool"
	modelport "myai/core/port/model"
	"myai/core/session"
	tooldef "myai/core/tool/tool"
)

type Registry interface {
	GetTool(name string) (tooldef.Tool, error)
}

// Invoker runs the resolved tool and may return trusted image-read metadata.
// Implementations must recognize the concrete built-in, never its name or JSON.
type Invoker interface {
	Invoke(ctx context.Context, registered tooldef.Tool, args json.RawMessage) (domaintool.ToolOutput, tooldef.ImageAssetID, error)
}

type AssetExtractor interface {
	Extract(command toolcommand.AssetExtraction) (domaintool.SharedAsset, bool)
}

type HookBridge interface {
	BeforeToolUse(ctx context.Context, event toolcommand.HookEvent) (toolresult.Hook, error)
	AfterToolUse(ctx context.Context, event toolcommand.HookEvent)
}

type LLMToolCatalog interface {
	LLMToolsByPermission(allow func(tooldef.Permission) bool) []modelport.Tool
}

type ToolModePolicy interface {
	AllowsToolPermission(permission tooldef.Permission, agentMode session.AgentMode, forceChatMode bool) bool
}
