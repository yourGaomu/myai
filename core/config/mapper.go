package config

import (
	"time"

	"myai/core/asset"
	domainmodel "myai/core/domain/model"
	"myai/core/hook"
	"myai/core/mcp"
)

type Mapper struct {
	Now func() time.Time
}

func (m Mapper) ModelConfig(properties ModelProperties) domainmodel.Config {
	now := m.now()
	return domainmodel.Config{
		ID:        properties.ID,
		Name:      properties.ID,
		Provider:  "openai",
		BaseURL:   properties.BaseURL,
		APIKey:    properties.APIKey,
		ModelName: properties.ID,
		Enabled:   true,
		IsDefault: true,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (Mapper) AssetConfig(properties AssetProperties) asset.Config {
	return asset.Config{
		BaseURL:           properties.BaseURL,
		Timeout:           time.Duration(properties.UploadTimeoutSeconds) * time.Second,
		DefaultTTLSeconds: properties.TTLSeconds,
		DefaultMaxVisits:  properties.MaxVisits,
	}
}

func (Mapper) HookConfig(workspace string, properties HookProperties) hook.Config {
	commands := make([]hook.CommandHookConfig, 0, len(properties.Commands))
	for _, command := range properties.Commands {
		commands = append(commands, hook.CommandHookConfig{
			Event:   command.Event,
			Command: command.Command,
			Timeout: command.Timeout,
			WorkDir: command.WorkDir,
			Enabled: command.Enabled,
		})
	}
	return hook.Config{
		Workspace:    workspace,
		CommandHooks: commands,
	}
}

func (Mapper) MCPConfig(workspace string, properties MCPProperties) mcp.Config {
	servers := make([]mcp.ServerConfig, 0, len(properties.Servers))
	for _, server := range properties.Servers {
		servers = append(servers, mcp.ServerConfig{
			Name:            server.Name,
			Command:         server.Command,
			Args:            append([]string(nil), server.Args...),
			Env:             cloneStringMap(server.Env),
			WorkingDir:      server.WorkingDir,
			Permission:      server.Permission,
			TimeoutSeconds:  server.TimeoutSeconds,
			ProtocolVersion: server.ProtocolVersion,
			Disabled:        server.Disabled,
			Required:        server.Required,
		})
	}
	return mcp.NormalizeConfig(mcp.Config{Servers: servers}, workspace)
}

func (m Mapper) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
