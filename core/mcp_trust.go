package core

import (
	"errors"

	appconfig "myai/core/config"
	"myai/core/mcp"
)

func (app *Application) MCPTrustWorkspace() string { return app.mcpWorkspace }

func (app *Application) MCPTrustCandidates(allowEndpoint func(string) bool) ([]mcp.MobileTrustCandidate, error) {
	if app.mcpManager == nil || app.mcpWorkspace == "" {
		return nil, errors.New("mcp manager is not configured")
	}
	properties, err := (appconfig.ViperLoader{ConfigFile: app.configPath}).LoadMCP()
	if err != nil {
		return nil, err
	}
	config := (appconfig.Mapper{}).MCPConfig(app.mcpWorkspace, properties)
	return mcp.MobileTrustCandidates(config, app.mcpWorkspace, allowEndpoint), nil
}

func (app *Application) MCPTrustRuntime(name, digest string) string {
	return app.mcpManager.MobileTrustRuntime(name, digest)
}
