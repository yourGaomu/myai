package sqlitevec

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func DefaultPath(workspace string) (string, error) {
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(absolute))))
	base, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(base) == "" {
		base, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(base, ".myai")
	} else {
		base = filepath.Join(base, "myai")
	}
	return filepath.Join(base, "workspaces", hex.EncodeToString(digest[:16]), "knowledge-vectors.db"), nil
}

func PathBeside(keywordPath string) (string, error) {
	if strings.TrimSpace(keywordPath) == "" {
		return "", fmt.Errorf("keyword database path is required")
	}
	absolute, err := filepath.Abs(strings.TrimSpace(keywordPath))
	if err != nil {
		return "", err
	}
	extension := filepath.Ext(absolute)
	base := strings.TrimSuffix(absolute, extension)
	if extension == "" {
		extension = ".db"
	}
	return base + "-vectors" + extension, nil
}

func ensureParentDirectory(path string) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create sqlite-vec directory %q: %w", directory, err)
	}
	return nil
}
