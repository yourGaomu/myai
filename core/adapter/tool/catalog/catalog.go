package catalog

import (
	runtimeservice "myai/core/application/runtime/service"
	toolport "myai/core/application/tool/port"
	toolservice "myai/core/application/tool/service"
	modelport "myai/core/port/model"
	"myai/core/session"
)

type Catalog struct {
	Tools      toolport.LLMToolCatalog
	ModePolicy toolport.ToolModePolicy
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
