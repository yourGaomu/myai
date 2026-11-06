package mcp

import (
	"path/filepath"
	"strings"
	"time"

	tooldef "myai/core/tool/tool"
	"myai/core/trust"
)

const (
	defaultProtocolVersion = "2025-06-18"
	defaultTimeout         = 30 * time.Second
)

type Config struct {
	Servers      []ServerConfig
	SourcePrefix string
}

type ServerConfig struct {
	Name            string
	URL             string
	Headers         map[string]string
	Command         string
	Args            []string
	Env             map[string]string
	WorkingDir      string
	Permission      string
	TimeoutSeconds  int
	ProtocolVersion string
	Disabled        bool
	Required        bool
	// Trust is injected by trusted application wiring, never workspace decoding.
	Trust          *trust.Policy `json:"-" yaml:"-" mapstructure:"-"`
	TrustWorkspace string        `json:"-" yaml:"-" mapstructure:"-"`
}

func NormalizeConfig(config Config, workspace string) Config {
	normalized := Config{Servers: append([]ServerConfig(nil), config.Servers...), SourcePrefix: config.SourcePrefix}
	for index := range normalized.Servers {
		normalized.Servers[index].Args = append([]string(nil), normalized.Servers[index].Args...)
		normalized.Servers[index].Env = cloneEnvironment(normalized.Servers[index].Env)
		normalized.Servers[index].Headers = cloneEnvironment(normalized.Servers[index].Headers)
		if policy := normalized.Servers[index].Trust; policy != nil {
			cloned := *policy
			normalized.Servers[index].Trust = &cloned
		}
		normalizeServerConfig(&normalized.Servers[index], workspace)
	}
	return normalized
}

func normalizeServerConfig(config *ServerConfig, workspace string) {
	config.Name = strings.TrimSpace(config.Name)
	config.URL = strings.TrimSpace(config.URL)
	config.Command = strings.TrimSpace(config.Command)
	config.WorkingDir = strings.TrimSpace(config.WorkingDir)
	config.Permission = strings.TrimSpace(strings.ToLower(config.Permission))
	config.ProtocolVersion = strings.TrimSpace(config.ProtocolVersion)

	if config.Permission == "" {
		config.Permission = string(tooldef.PermissionExecute)
	}
	if config.TimeoutSeconds <= 0 {
		config.TimeoutSeconds = int(defaultTimeout.Seconds())
	}
	if config.ProtocolVersion == "" {
		config.ProtocolVersion = defaultProtocolVersion
	}

	if config.WorkingDir == "" {
		config.WorkingDir = workspace
	}
	if config.WorkingDir != "" && !filepath.IsAbs(config.WorkingDir) && workspace != "" {
		config.WorkingDir = filepath.Join(workspace, config.WorkingDir)
	}
}

func (config ServerConfig) timeout() time.Duration {
	if config.TimeoutSeconds <= 0 {
		return defaultTimeout
	}
	return time.Duration(config.TimeoutSeconds) * time.Second
}

func (config ServerConfig) toolPermission() tooldef.Permission {
	return tooldef.NormalizePermission(tooldef.Permission(strings.ToLower(strings.TrimSpace(config.Permission))))
}

func cloneEnvironment(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
