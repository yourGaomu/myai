package mcp

import (
	"errors"
	"net/url"
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
	if strings.TrimSpace(workspace) != "" {
		if absolute, err := filepath.Abs(workspace); err == nil {
			workspace = absolute
		}
	}
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

// TrustRequest returns the normalized declaration used by both startup and the
// trust CLI. It resolves the HTTP endpoint, but never expands header or stdio
// environment values. Approval stays valid when those values rotate.
func TrustRequest(config ServerConfig, workspace string) (trust.Request, error) {
	if strings.TrimSpace(workspace) == "" {
		workspace = config.TrustWorkspace
	}
	config = NormalizeConfig(Config{Servers: []ServerConfig{config}}, workspace).Servers[0]
	request, _, err := prepareTrustRequest(config, workspace)
	return request, err
}

func prepareTrustRequest(config ServerConfig, workspace string) (trust.Request, string, error) {
	if config.Command == "" && config.URL == "" {
		return trust.Request{}, "", errors.New("mcp server requires command or url")
	}
	var endpoint string
	if config.URL != "" {
		if config.Command != "" {
			return trust.Request{}, "", errors.New("mcp server must configure either url or command, not both")
		}
		var err error
		endpoint, err = resolveHTTPEndpoint(config.URL)
		if err != nil {
			return trust.Request{}, "", err
		}
	}
	if strings.TrimSpace(workspace) == "" {
		return trust.Request{}, "", errors.New("mcp trust workspace is required")
	}
	workspace, err := filepath.Abs(workspace)
	if err != nil {
		return trust.Request{}, "", errors.New("cannot resolve mcp trust workspace")
	}
	declaration := struct {
		Name            string            `json:"name"`
		URL             string            `json:"url"`
		Headers         map[string]string `json:"headers"`
		Command         string            `json:"command"`
		Args            []string          `json:"args"`
		Env             map[string]string `json:"env"`
		WorkingDir      string            `json:"working_dir"`
		Permission      string            `json:"permission"`
		TimeoutSeconds  int               `json:"timeout_seconds"`
		ProtocolVersion string            `json:"protocol_version"`
		Disabled        bool              `json:"disabled"`
		Required        bool              `json:"required"`
		ResolvedURL     string            `json:"resolved_url,omitempty"`
	}{config.Name, config.URL, config.Headers, config.Command, config.Args,
		config.Env, config.WorkingDir, config.Permission, config.TimeoutSeconds,
		config.ProtocolVersion, config.Disabled, config.Required, endpoint}
	request, err := trust.NewRequest(trust.KindMCP, workspace, workspace, declaration)
	if err != nil {
		return trust.Request{}, "", errors.New("cannot prepare mcp trust declaration")
	}
	return request, endpoint, nil
}

func resolveHTTPEndpoint(template string) (string, error) {
	endpoint, err := expandHTTPEnvironment(template)
	if err != nil {
		return "", errors.New("mcp HTTP endpoint environment reference is not configured")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Opaque != "" || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || strings.Contains(endpoint, "#") {
		return "", errors.New("mcp HTTP url must be an absolute http(s) URL without userinfo or fragment")
	}
	return parsed.String(), nil
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
