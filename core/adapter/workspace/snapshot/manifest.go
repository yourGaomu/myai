package snapshot

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	domainworkspace "myai/core/domain/workspace"
)

const manifestFileName = "manifest.json"

type fileState struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
	Mode uint32 `json:"mode"`
}

type manifest struct {
	Version       int                             `json:"version"`
	WorkspaceID   string                          `json:"workspace_id"`
	SourceRoot    string                          `json:"source_root"`
	SnapshotRoot  string                          `json:"snapshot_root"`
	Status        domainworkspace.ChangeSetStatus `json:"status"`
	CheckpointID  string                          `json:"checkpoint_id,omitempty"`
	CreatedAt     time.Time                       `json:"created_at"`
	AppliedAt     *time.Time                      `json:"applied_at,omitempty"`
	BaselineFiles []fileState                     `json:"baseline_files"`
}

func loadManifest(path string) (manifest, error) {
	content, err := os.ReadFile(filepath.Join(path, manifestFileName))
	if err != nil {
		return manifest{}, err
	}
	var result manifest
	if err := json.Unmarshal(content, &result); err != nil {
		return manifest{}, err
	}
	if result.Version != 1 || result.WorkspaceID == "" || result.SourceRoot == "" || result.SnapshotRoot == "" {
		return manifest{}, errors.New("invalid snapshot workspace manifest")
	}
	return result, nil
}

func saveManifest(path string, value manifest) error {
	return saveManifestWithReplace(path, value, replaceFile)
}

func saveManifestWithReplace(path string, value manifest, replace func(string, string) error) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(path, ".manifest-*.json")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	destination := filepath.Join(path, manifestFileName)
	return replace(temporaryPath, destination)
}

func stateMap(files []fileState) map[string]fileState {
	result := make(map[string]fileState, len(files))
	for _, file := range files {
		result[file.Path] = file
	}
	return result
}
