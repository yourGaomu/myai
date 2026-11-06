// Package pathpolicy classifies workspace paths without reading file contents.
package pathpolicy

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Policy also protects installation roots that do not use conventional names.
// ProtectedRoots may contain workspace-relative or absolute paths.
type Policy struct {
	WorkspaceRoot  string
	ProtectedRoots []string
}

type ResolvedPath struct {
	Abs      string
	Relative string
}

// IsSensitive reports paths that may contain credentials or private configuration.
// Both slash styles and case variants are recognized on every platform.
func IsSensitive(path string) bool {
	if userConfigPath(path) {
		return true
	}
	parts := components(path)
	for i, part := range parts {
		if part == "myai" && i+1 < len(parts) && parts[i+1] == "trust.json" {
			return true
		}
		if sensitiveDirectory(part) || sensitiveFile(part) {
			return true
		}
	}
	return false
}

// IsProtected also includes repository metadata and agent executable declarations.
func IsProtected(path string) bool {
	if IsSensitive(path) {
		return true
	}
	for _, part := range components(path) {
		if protectedDirectory(part) {
			return true
		}
		switch part {
		case "agents.md", "claude.md", "skill.md", "plugin.json", "hooks.json", "mcp.json", ".mcp.json":
			return true
		}
	}
	return false
}

// ShouldSkipDirectory is intended for directory walkers, not file predicates.
func ShouldSkipDirectory(path string) bool {
	if userConfigPath(path) {
		return true
	}
	for _, part := range components(path) {
		if sensitiveDirectory(part) || protectedDirectory(part) || cacheDirectory(part) {
			return true
		}
	}
	return false
}

// ShouldSkipSnapshot excludes credentials, agent state and generated caches.
func ShouldSkipSnapshot(path string) bool {
	return IsProtected(path) || ShouldSkipDirectory(path)
}

// Resolve returns a canonical path inside workspace. Existing symlink ancestors
// are resolved even when the final file does not exist yet.
func Resolve(workspace, raw string) (string, error) {
	resolved, err := (Policy{WorkspaceRoot: workspace}).Resolve(raw)
	return resolved.Abs, err
}

func (p Policy) Resolve(raw string) (ResolvedPath, error) {
	if strings.TrimSpace(p.WorkspaceRoot) == "" || strings.TrimSpace(raw) == "" {
		return ResolvedPath{}, errors.New("workspace and path are required")
	}
	if ambiguousPath(raw) || ambiguousPath(p.WorkspaceRoot) {
		return ResolvedPath{}, errors.New("ambiguous workspace path")
	}
	root, err := canonical(p.WorkspaceRoot)
	if err != nil {
		return ResolvedPath{}, errors.New("cannot resolve workspace")
	}
	if unsafeWorkspaceRoot(p.WorkspaceRoot) || unsafeWorkspaceRoot(root) {
		return ResolvedPath{}, errors.New("workspace root is protected user or repository metadata")
	}
	target := nativePath(raw)
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	// Reject a lexical escape as well as an escape through a symlink.
	if _, ok := relativeWithin(root, target); !ok {
		return ResolvedPath{}, errors.New("path is outside workspace")
	}
	target, err = canonical(target)
	if err != nil {
		return ResolvedPath{}, errors.New("cannot resolve workspace path")
	}
	relative, ok := relativeWithin(root, target)
	if !ok {
		return ResolvedPath{}, errors.New("path is outside workspace")
	}
	return ResolvedPath{Abs: target, Relative: filepath.ToSlash(relative)}, nil
}

func (p Policy) RequiresApproval(path string) bool {
	original, err := p.relativeOriginal(path)
	if err != nil || IsProtected(original) {
		return true
	}
	resolved, err := p.Resolve(path)
	if err != nil || IsProtected(resolved.Relative) || userConfigPath(resolved.Abs) {
		return true
	}
	for _, root := range p.ProtectedRoots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		if !filepath.IsAbs(nativePath(root)) {
			root = filepath.Join(p.WorkspaceRoot, nativePath(root))
		}
		root, err := canonical(root)
		if err != nil {
			return true
		}
		if _, ok := relativeWithin(root, resolved.Abs); ok {
			return true
		}
	}
	return false
}

func (p Policy) Sensitive(path string) bool {
	original, err := p.relativeOriginal(path)
	if err != nil || IsSensitive(original) {
		return true
	}
	resolved, err := p.Resolve(path)
	return err != nil || IsSensitive(resolved.Relative) || userConfigPath(resolved.Abs)
}

func (p Policy) SkipSnapshot(path string) bool {
	original, err := p.relativeOriginal(path)
	if err != nil || ShouldSkipSnapshot(original) || p.RequiresApproval(path) {
		return true
	}
	resolved, err := p.Resolve(path)
	return err != nil || ShouldSkipSnapshot(resolved.Relative)
}

// Classify the submitted path relative to this workspace before cleaning it.
// This preserves protected aliases such as .git/../report.txt while avoiding
// classification of a child workspace's .myai snapshot storage ancestors.
func (p Policy) relativeOriginal(path string) (string, error) {
	if !filepath.IsAbs(nativePath(path)) {
		return path, nil
	}
	root, err := filepath.Abs(nativePath(p.WorkspaceRoot))
	if err != nil {
		return "", err
	}
	roots := []string{root}
	if resolved, err := canonical(root); err == nil {
		roots = append(roots, resolved)
	}
	path = nativePath(path)
	for _, root := range roots {
		root = strings.TrimRight(root, string(filepath.Separator))
		comparisonPath, comparisonRoot := path, root
		if runtime.GOOS == "windows" {
			comparisonPath, comparisonRoot = strings.ToLower(path), strings.ToLower(root)
		}
		if comparisonPath == comparisonRoot {
			return ".", nil
		}
		if strings.HasPrefix(comparisonPath, comparisonRoot+string(filepath.Separator)) {
			return path[len(root)+1:], nil
		}
	}
	return "", errors.New("path is outside workspace")
}

func unsafeWorkspaceRoot(path string) bool {
	if userConfigPath(path) {
		return true
	}
	for _, part := range components(path) {
		if part == ".git" {
			return true
		}
	}
	return false
}

func userConfigPath(path string) bool {
	parts := components(path)
	for i, part := range parts {
		if part == "myai" && i > 0 && (parts[i-1] == "roaming" || parts[i-1] == ".config") {
			return true
		}
	}
	if !filepath.IsAbs(nativePath(path)) {
		return false
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return false
	}
	root = filepath.Join(root, "myai")
	if _, ok := relativeWithin(root, nativePath(path)); ok {
		return true
	}
	resolvedRoot, rootErr := canonical(root)
	resolvedPath, pathErr := canonical(path)
	if rootErr != nil || pathErr != nil {
		return false
	}
	_, ok := relativeWithin(resolvedRoot, resolvedPath)
	return ok
}

func components(path string) []string {
	parts := strings.FieldsFunc(strings.ToLower(path), func(r rune) bool { return r == '/' || r == '\\' })
	for i := range parts {
		// Windows ignores trailing dots/spaces and supports alternate streams.
		parts[i] = strings.TrimRight(strings.SplitN(parts[i], ":", 2)[0], ". ")
	}
	return parts
}

func sensitiveDirectory(name string) bool {
	switch name {
	case "config", "configs", "configuration", ".config", "secrets", ".secrets", "credentials", ".credentials",
		"privatekeys", "private_keys", "private-keys", ".ssh", ".aws", ".azure", ".kube", ".gnupg", ".docker":
		return true
	}
	return false
}

func sensitiveFile(name string) bool {
	if name == ".env" || strings.HasPrefix(name, ".env.") || strings.HasPrefix(name, ".env-") {
		return true
	}
	switch name {
	case ".netrc", "_netrc", ".npmrc", ".pypirc", ".git-credentials", ".pgpass", "id_rsa", "id_dsa", "id_ecdsa", "id_ed25519":
		return true
	}
	for _, prefix := range []string{"application.", "relay.", "credentials.", "secrets.", "privatekey.", "private_key.", "config.", "settings."} {
		if strings.HasPrefix(name, prefix) {
			switch filepath.Ext(name) {
			case ".yaml", ".yml", ".json", ".toml", ".ini", ".conf", ".xml", ".properties", ".key", ".pem":
				return true
			}
		}
	}
	switch filepath.Ext(name) {
	case ".pem", ".key", ".p12", ".pfx", ".keystore", ".jks":
		return true
	}
	return false
}

func protectedDirectory(name string) bool {
	switch name {
	case ".git", ".myai", ".zcode", ".claude", ".codex", ".vscode", "skill", "skills", "plugin", "plugins", "hooks":
		return true
	}
	return false
}

func cacheDirectory(name string) bool {
	switch name {
	case "node_modules", "vendor", ".cache", "__pycache__", ".venv", "venv", ".pytest_cache", ".mypy_cache",
		".idea", ".expo", "dist", "build", ".next":
		return true
	}
	return false
}

func nativePath(path string) string {
	return filepath.FromSlash(strings.ReplaceAll(path, "\\", "/"))
}

func ambiguousPath(path string) bool {
	if strings.ContainsRune(path, '\x00') || strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) {
		return true
	}
	for _, part := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == "." || part == ".." {
			continue
		}
		if strings.TrimRight(part, ". ") != part {
			return true
		}
		if strings.Contains(part, ":") && !(len(part) == 2 && part[1] == ':' && runtime.GOOS == "windows") {
			return true
		}
	}
	return false
}

func relativeWithin(root, path string) (string, bool) {
	if runtime.GOOS == "windows" {
		root, path = strings.ToLower(root), strings.ToLower(path)
	}
	relative, err := filepath.Rel(root, filepath.Clean(path))
	return relative, err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func canonical(path string) (string, error) {
	path, err := filepath.Abs(nativePath(path))
	if err != nil {
		return "", err
	}
	var suffix []string
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) || filepath.Dir(current) == current {
			return "", err
		}
		suffix = append(suffix, filepath.Base(current))
	}
}
