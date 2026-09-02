package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	mcpserver "myai/core/mcp"
)

const (
	ManifestFileName      = "plugin.json"
	maxManifestBytes      = 256 * 1024
	defaultPluginProtocol = "mcp"
	defaultPluginVersion  = "0.1.0"
)

var pluginIDPattern = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$")

// Manifest is the local plugin contract. The first implementation supports
// MCP-compatible executable plugins; additional capabilities can be added
// without changing the discovery and lifecycle contract.
type Manifest struct {
	ID             string            `json:"id"`
	Name           string            `json:"name,omitempty"`
	Version        string            `json:"version,omitempty"`
	Protocol       string            `json:"protocol,omitempty"`
	Type           string            `json:"type,omitempty"`
	Entrypoint     string            `json:"entrypoint"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	WorkingDir     string            `json:"working_dir,omitempty"`
	Permission     string            `json:"permission,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	Enabled        *bool             `json:"enabled,omitempty"`
	Required       bool              `json:"required,omitempty"`
}

func (manifest Manifest) normalized() Manifest {
	manifest.ID = strings.TrimSpace(manifest.ID)
	manifest.Name = strings.TrimSpace(manifest.Name)
	manifest.Version = strings.TrimSpace(manifest.Version)
	manifest.Protocol = strings.ToLower(strings.TrimSpace(manifest.Protocol))
	manifest.Type = strings.ToLower(strings.TrimSpace(manifest.Type))
	manifest.Entrypoint = strings.TrimSpace(manifest.Entrypoint)
	manifest.WorkingDir = strings.TrimSpace(manifest.WorkingDir)
	manifest.Permission = strings.ToLower(strings.TrimSpace(manifest.Permission))
	if manifest.Name == "" {
		manifest.Name = manifest.ID
	}
	if manifest.Version == "" {
		manifest.Version = defaultPluginVersion
	}
	if manifest.Protocol == "" {
		manifest.Protocol = manifest.Type
	}
	if manifest.Protocol == "" {
		manifest.Protocol = defaultPluginProtocol
	}
	if manifest.Permission == "" {
		manifest.Permission = "read"
	}
	if manifest.TimeoutSeconds <= 0 {
		manifest.TimeoutSeconds = 30
	}
	if manifest.Args != nil {
		manifest.Args = append([]string(nil), manifest.Args...)
	}
	if manifest.Env != nil {
		env := make(map[string]string, len(manifest.Env))
		for key, value := range manifest.Env {
			env[key] = value
		}
		manifest.Env = env
	}
	return manifest
}

func (manifest Manifest) validate() error {
	if manifest.ID == "" {
		return errors.New("plugin id is required")
	}
	if !pluginIDPattern.MatchString(manifest.ID) {
		return fmt.Errorf("plugin id %q is invalid", manifest.ID)
	}
	if manifest.Name == "" {
		return errors.New("plugin name is required")
	}
	if manifest.Protocol != defaultPluginProtocol {
		return fmt.Errorf("plugin %s uses unsupported protocol %q", manifest.ID, manifest.Protocol)
	}
	if manifest.Entrypoint == "" {
		return errors.New("plugin entrypoint is required")
	}
	if manifest.TimeoutSeconds < 1 || manifest.TimeoutSeconds > 3600 {
		return fmt.Errorf("plugin timeout must be between 1 and 3600 seconds")
	}
	return nil
}

func loadManifest(directory string) (Manifest, error) {
	path := filepath.Join(directory, ManifestFileName)
	info, err := os.Stat(path)
	if err != nil {
		return Manifest{}, err
	}
	if info.Size() > maxManifestBytes {
		return Manifest{}, fmt.Errorf("plugin manifest exceeds %d bytes", maxManifestBytes)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode %s: %w", path, err)
	}
	manifest = manifest.normalized()
	if err := manifest.validate(); err != nil {
		return Manifest{}, fmt.Errorf("validate %s: %w", path, err)
	}
	return manifest, nil
}

func (manifest Manifest) enabled() bool {
	return manifest.Enabled == nil || *manifest.Enabled
}

func (manifest Manifest) serverConfig(directory string) mcpserver.ServerConfig {
	command := manifest.Entrypoint
	if !filepath.IsAbs(command) {
		candidate := filepath.Join(directory, command)
		if _, err := os.Stat(candidate); err == nil {
			command = candidate
		}
	}
	workingDir := manifest.WorkingDir
	if workingDir == "" {
		workingDir = directory
	} else if !filepath.IsAbs(workingDir) {
		workingDir = filepath.Join(directory, workingDir)
	}
	return mcpserver.NormalizeConfig(mcpserver.Config{Servers: []mcpserver.ServerConfig{{
		Name:            manifest.ID,
		Command:         command,
		Args:            append([]string(nil), manifest.Args...),
		Env:             cloneMap(manifest.Env),
		WorkingDir:      workingDir,
		Permission:      manifest.Permission,
		TimeoutSeconds:  manifest.TimeoutSeconds,
		ProtocolVersion: "",
		Required:        manifest.Required,
	}}}, directory).Servers[0]
}

func cloneMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
