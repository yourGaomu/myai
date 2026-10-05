package catalog

import (
	"context"
	generationport "myai/core/application/chat/generation/port"
	runtimeservice "myai/core/application/runtime/service"
	toolport "myai/core/application/tool/port"
	toolruntime "myai/core/application/tool/runtime"
	toolservice "myai/core/application/tool/service"
	modelport "myai/core/port/model"
	"myai/core/session"
	"myai/core/tool"
)

type Catalog struct {
	Tools      toolport.LLMToolCatalog
	ModePolicy toolport.ToolModePolicy
}

func (c Catalog) BeginTurn(ctx context.Context) (context.Context, generationport.ToolCatalog, func()) {
	if registry, ok := c.Tools.(interface {
		Snapshot() (*tool.RegisterTools, func())
	}); ok {
		snapshot, release := registry.Snapshot()
		c.Tools = snapshot
		return toolruntime.WithRegistry(ctx, snapshot), c, release
	}
	return ctx, c, func() {}
}

func (c Catalog) ToolsForSession(current *session.Session, forceChatMode bool) []modelport.Tool {
	return toolservice.SelectionService{
		Catalog:    c.Tools,
		ModePolicy: c.modePolicy(),
	}.ToolsForSession(current, forceChatMode)
}

func (c Catalog) modePolicy() toolport.ToolModePolicy {
	if c.ModePolicy != nil {
		return c.ModePolicy
	}
	return runtimeservice.ModePolicy{}
}
