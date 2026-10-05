package agent

import "context"

// MCPManagerFacade exposes only the atomic configuration reload needed by the
// remote transport. The Application owns config loading and tool registry
// wiring; the relay never receives those implementation details.
type MCPManagerFacade interface {
	ReloadMCP(context.Context) error
}
