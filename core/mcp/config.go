package mcp

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"

	tooldef "myai/core/tool/tool"
)

const (
	defaultProtocolVersion = "2025-06-18"
	defaultTimeout         = 30 * time.Second
)

type Config struct {
	Servers []ServerConfig `mapstructure:"servers"`
}

type ServerConfig struct {
	Name            string            `mapstructure:"name"`
	Command         string            `mapstructure:"command"`
	Args            []string          `mapstructure:"args"`
	Env             map[string]string `mapstructure:"env"`
	WorkingDir      string            `mapstructure:"working_dir"`
	Permission      string            `mapstructure:"permission"`
	TimeoutSeconds  int               `mapstructure:"timeout_seconds"`
	ProtocolVersion string            `mapstructure:"protocol_version"`
	Disabled        bool              `mapstructure:"disabled"`
	Required        bool              `mapstructure:"required"`
}

func LoadConfig(v *viper.Viper, workspace string) (Config, error) {
	var cfg Config
	if v == nil || !v.IsSet("mcp") {
		return cfg, nil
	}

	if err := v.UnmarshalKey("mcp", &cfg); err != nil {
		return Config{}, err
	}

	for index := range cfg.Servers {
		normalizeServerConfig(&cfg.Servers[index], workspace)
	}
	return cfg, nil
}

func normalizeServerConfig(config *ServerConfig, workspace string) {
	config.Name = strings.TrimSpace(config.Name)
	config.Command = strings.TrimSpace(config.Command)
	config.WorkingDir = strings.TrimSpace(config.WorkingDir)
	config.Permission = strings.TrimSpace(strings.ToLower(config.Permission))
	config.ProtocolVersion = strings.TrimSpace(config.ProtocolVersion)

	if config.Permission == "" {
		config.Permission = string(tooldef.PermissionRead)
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
