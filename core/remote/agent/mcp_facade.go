package agent

import (
	"context"

	"myai/core/mcp"
)

// MCPManagerFacade delegates reload to the Application, which owns the configuration and registry.
type MCPManagerFacade interface {
	ReloadMCP(context.Context) error
}

type mcpStatusReader interface {
	MCPServerStatuses() ([]mcp.ServerStatus, error)
}

type mcpCatalogStatusReader interface {
	MCPCatalogStatuses() ([]mcp.ServerStatus, uint64, error)
}

type mcpDetailReader interface {
	MCPServerDetail(serverName, sessionID, cursor string) (mcp.ServerDetail, error)
}
