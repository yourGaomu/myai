package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
		manifest.Permission = "execute"
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
	if manifest.Permission != "read" && manifest.Permission != "write" && manifest.Permission != "execute" {
		return errors.New("plugin permission must be read, write, or execute")
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
	return decodeManifest(path, raw)
}

func decodeManifest(path string, raw []byte) (Manifest, error) {
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

// Read only the regular manifest that an update would replace. In particular,
// renaming over a symlink would update a different file than the one parsed.
func readManifestForUpdate(path string) ([]byte, os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, errors.New("plugin manifest must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !os.SameFile(info, opened) {
		return nil, nil, errors.New("plugin manifest changed during update")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxManifestBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if len(raw) > maxManifestBytes {
		return nil, nil, fmt.Errorf("plugin manifest exceeds %d bytes", maxManifestBytes)
	}
	return raw, info, nil
}

func replaceManifest(path string, original, replacement []byte) error {
	if len(replacement) > maxManifestBytes {
		return fmt.Errorf("plugin manifest exceeds %d bytes", maxManifestBytes)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".plugin-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	current, info, err := readManifestForUpdate(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, original) {
		return errors.New("plugin manifest changed during update")
	}
	if err := file.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if _, err := file.Write(replacement); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	current, latest, err := readManifestForUpdate(path)
	if err != nil {
		return err
	}
	if !os.SameFile(info, latest) || !bytes.Equal(current, original) {
		return errors.New("plugin manifest changed during update")
	}
	return os.Rename(file.Name(), path)
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
