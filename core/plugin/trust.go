package plugin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"myai/core/mcp"
	"myai/core/trust"
)

// TrustRequest binds the complete normalized manifest and resolved launch paths
// to a workspace. Environment values participate in the digest but are never
// written to the user's approval file.
func TrustRequest(manifest Manifest, workspace, directory string) (trust.Request, error) {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return trust.Request{}, err
	}
	manifest = manifest.normalized()
	return launchTrustRequest(manifest, workspace, directory, manifest.serverConfig(directory))
}

func launchTrustRequest(manifest Manifest, workspace, directory string, server mcp.ServerConfig) (trust.Request, error) {
	return trust.NewRequest(trust.KindPlugin, workspace, filepath.Join(directory, ManifestFileName), struct {
		Manifest   Manifest `json:"manifest"`
		Command    string   `json:"command"`
		WorkingDir string   `json:"working_dir"`
	}{manifest, server.Command, server.WorkingDir})
}

// ReviewRequests discovers declarations for the user-facing approval command. It
// never connects to a plugin or starts its executable.
func ReviewRequests(root, workspace string) ([]trust.Request, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("scan plugin declarations")
	}
	var requests []trust.Request
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		directory := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(directory, ManifestFileName)); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, errors.New("read plugin declaration")
		}
		manifest, err := loadManifest(directory)
		if err != nil {
			return nil, fmt.Errorf("invalid plugin manifest in %s", directory)
		}
		request, err := TrustRequest(manifest, workspace, directory)
		if err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, nil
}
