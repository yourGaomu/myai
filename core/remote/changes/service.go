package changes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"myai/core/remote/protocol"
)

const (
	defaultChangeLimit = 200
	maxChangeLimit     = 1000
	maxDiffBytes       = 256 * 1024
	gitTimeout         = 10 * time.Second
)

var ignoredNames = map[string]bool{
	".git":         true,
	".idea":        true,
	".expo":        true,
	"node_modules": true,
	"dist":         true,
	"build":        true,
	".next":        true,
	".cache":       true,
}

var sensitiveNames = map[string]bool{
	".env":            true,
	".env.local":      true,
	".env.production": true,
	"id_rsa":          true,
	"id_ed25519":      true,
}

type Service struct {
	root string
}

func New(root string) (*Service, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace is not a directory: %s", abs)
	}

	return &Service{root: abs}, nil
}

func (s *Service) List(ctx context.Context, payload protocol.ChangesListPayload) (protocol.ChangesListResultPayload, error) {
	repoRoot, err := s.repoRoot(ctx)
	if err != nil {
		return protocol.ChangesListResultPayload{
			Repository: false,
			Entries:    []protocol.ChangeEntry{},
			Clean:      true,
			Message:    "workspace is not a git repository",
		}, nil
	}

	limit := payload.Limit
	if limit <= 0 {
		limit = defaultChangeLimit
	}
	if limit > maxChangeLimit {
		limit = maxChangeLimit
	}

	workspacePathspec, err := repoRelativePath(repoRoot, s.root)
	if err != nil {
		return protocol.ChangesListResultPayload{}, err
	}

	args := []string{"status", "--porcelain=v1", "--untracked-files=normal"}
	if workspacePathspec != "." {
		args = append(args, "--", workspacePathspec)
	}
	output, _, err := runGit(ctx, repoRoot, 0, args...)
	if err != nil {
		return protocol.ChangesListResultPayload{}, err
	}

	entries := make([]protocol.ChangeEntry, 0)
	truncated := false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		entry, ok := parseStatusLine(line)
		if !ok {
			continue
		}
		entry, ok = entryToWorkspace(entry, repoRoot, s.root)
		if !ok || shouldHidePath(entry.Path) {
			continue
		}
		if len(entries) >= limit {
			truncated = true
			break
		}
		entries = append(entries, entry)
	}

	return protocol.ChangesListResultPayload{
		Repository: true,
		Root:       filepath.ToSlash(repoRoot),
		Entries:    entries,
		Count:      len(entries),
		Truncated:  truncated,
		Clean:      len(entries) == 0,
	}, nil
}

func (s *Service) Diff(ctx context.Context, payload protocol.ChangeDiffPayload) (protocol.ChangeDiffResultPayload, error) {
	repoRoot, err := s.repoRoot(ctx)
	if err != nil {
		return protocol.ChangeDiffResultPayload{
			Path:    payload.Path,
			Message: "workspace is not a git repository",
		}, nil
	}

	rel, abs, err := cleanPath(s.root, payload.Path)
	if err != nil {
		return protocol.ChangeDiffResultPayload{}, err
	}
	if shouldHidePath(rel) {
		return protocol.ChangeDiffResultPayload{}, fmt.Errorf("refusing to preview sensitive change: %s", rel)
	}

	repoRel, err := repoRelativePath(repoRoot, abs)
	if err != nil {
		return protocol.ChangeDiffResultPayload{}, err
	}

	staged, stagedTruncated, err := runGit(ctx, repoRoot, maxDiffBytes, "diff", "--cached", "--no-color", "--", repoRel)
	if err != nil {
		return protocol.ChangeDiffResultPayload{}, err
	}
	unstaged, unstagedTruncated, err := runGit(ctx, repoRoot, maxDiffBytes, "diff", "--no-color", "--", repoRel)
	if err != nil {
		return protocol.ChangeDiffResultPayload{}, err
	}

	diff := combineDiffs(staged, unstaged)
	truncated := stagedTruncated || unstagedTruncated
	if strings.TrimSpace(diff) == "" {
		untrackedDiff, untrackedTruncated, binary, err := s.untrackedDiff(abs, rel)
		if err != nil {
			return protocol.ChangeDiffResultPayload{}, err
		}
		diff = untrackedDiff
		truncated = untrackedTruncated
		return protocol.ChangeDiffResultPayload{
			Path:      rel,
			Diff:      diff,
			Truncated: truncated,
			Binary:    binary,
			Message:   emptyDiffMessage(diff, binary),
		}, nil
	}

	return protocol.ChangeDiffResultPayload{
		Path:      rel,
		Diff:      diff,
		Truncated: truncated,
		Binary:    false,
	}, nil
}

func (s *Service) repoRoot(ctx context.Context) (string, error) {
	output, _, err := runGit(ctx, s.root, 0, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}

	root := filepath.Clean(strings.TrimSpace(output))
	if root == "" {
		return "", errors.New("git repository root is empty")
	}
	return root, nil
}

func runGit(ctx context.Context, dir string, outputLimit int, args ...string) (string, bool, error) {
	runCtx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	allArgs := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(runCtx, "git", allArgs...)

	stdout := newLimitedBuffer(outputLimit)
	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return stdout.String(), stdout.Truncated(), fmt.Errorf("git %s failed: %s", strings.Join(args, " "), message)
	}

	return stdout.String(), stdout.Truncated(), nil
}

func parseStatusLine(line string) (protocol.ChangeEntry, bool) {
	if len(line) < 4 {
		return protocol.ChangeEntry{}, false
	}

	index := line[0]
	worktree := line[1]
	pathField := strings.TrimSpace(line[3:])
	if pathField == "" {
		return protocol.ChangeEntry{}, false
	}

	entry := protocol.ChangeEntry{
		Path:           cleanGitPath(pathField),
		Status:         statusLabel(index, worktree),
		IndexStatus:    string(index),
		WorktreeStatus: string(worktree),
		Staged:         isStaged(index),
		Unstaged:       isUnstaged(worktree),
		Untracked:      index == '?' && worktree == '?',
		Deleted:        index == 'D' || worktree == 'D',
		Renamed:        index == 'R' || worktree == 'R',
	}

	if strings.Contains(pathField, " -> ") {
		parts := strings.SplitN(pathField, " -> ", 2)
		entry.OldPath = cleanGitPath(parts[0])
		entry.Path = cleanGitPath(parts[1])
	}

	return entry, true
}

func cleanGitPath(path string) string {
	path = strings.Trim(path, `"`)
	path = strings.TrimSpace(path)
	return filepath.ToSlash(path)
}

func statusLabel(index byte, worktree byte) string {
	switch {
	case index == '?' && worktree == '?':
		return "untracked"
	case index == 'R' || worktree == 'R':
		return "renamed"
	case index == 'A' || worktree == 'A':
		return "added"
	case index == 'D' || worktree == 'D':
		return "deleted"
	case index == 'M' || worktree == 'M':
		return "modified"
	case index == 'C' || worktree == 'C':
		return "copied"
	default:
		return "changed"
	}
}

func isStaged(status byte) bool {
	return status != ' ' && status != '?' && status != '!'
}

func isUnstaged(status byte) bool {
	return status != ' ' && status != '?' && status != '!'
}

func combineDiffs(staged string, unstaged string) string {
	staged = strings.TrimRight(staged, "\r\n")
	unstaged = strings.TrimRight(unstaged, "\r\n")

	if staged == "" {
		return unstaged
	}
	if unstaged == "" {
		return staged
	}
	return "# Staged changes\n\n" + staged + "\n\n# Unstaged changes\n\n" + unstaged
}

func (s *Service) untrackedDiff(abs string, rel string) (string, bool, bool, error) {
	info, err := os.Stat(abs)
	if err != nil {
		return "", false, false, nil
	}
	if info.IsDir() {
		return "", false, false, nil
	}

	file, err := os.Open(abs)
	if err != nil {
		return "", false, false, err
	}
	defer file.Close()

	readLimit := maxDiffBytes
	if info.Size() < int64(readLimit) {
		readLimit = int(info.Size())
	}
	buffer := make([]byte, readLimit)
	n, err := io.ReadFull(file, buffer)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", false, false, err
	}

	content := buffer[:n]
	if isBinary(content) {
		return "", info.Size() > int64(maxDiffBytes), true, nil
	}

	text := string(content)
	lineCount := countLines(text)
	var builder strings.Builder
	builder.WriteString("diff --git a/")
	builder.WriteString(rel)
	builder.WriteString(" b/")
	builder.WriteString(rel)
	builder.WriteString("\nnew file mode 100644\n--- /dev/null\n+++ b/")
	builder.WriteString(rel)
	builder.WriteString(fmt.Sprintf("\n@@ -0,0 +1,%d @@\n", lineCount))
	for _, line := range strings.SplitAfter(text, "\n") {
		if line == "" {
			continue
		}
		builder.WriteString("+")
		builder.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			builder.WriteString("\n")
		}
	}

	diff := builder.String()
	truncated := info.Size() > int64(maxDiffBytes)
	if len(diff) > maxDiffBytes {
		diff = diff[:maxDiffBytes]
		truncated = true
	}
	return diff, truncated, false, nil
}

func countLines(text string) int {
	if text == "" {
		return 0
	}
	count := strings.Count(text, "\n")
	if !strings.HasSuffix(text, "\n") {
		count++
	}
	return count
}

func emptyDiffMessage(diff string, binary bool) string {
	if binary {
		return "Binary file diff is not available."
	}
	if strings.TrimSpace(diff) == "" {
		return "No diff is available for this path."
	}
	return ""
}

func cleanPath(root string, path string) (string, string, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || clean == "." {
		return "", "", errors.New("change path is empty")
	}
	if filepath.IsAbs(clean) {
		return "", "", errors.New("absolute paths are not allowed")
	}

	abs := filepath.Join(root, clean)
	abs = filepath.Clean(abs)
	if !insideRoot(root, abs) {
		return "", "", fmt.Errorf("path escapes workspace: %s", path)
	}

	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", "", err
	}
	return filepath.ToSlash(rel), abs, nil
}

func repoRelativePath(repoRoot string, target string) (string, error) {
	rel, err := filepath.Rel(repoRoot, target)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return ".", nil
	}
	if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path is outside git repository: %s", target)
	}
	return filepath.ToSlash(rel), nil
}

func entryToWorkspace(entry protocol.ChangeEntry, repoRoot string, workspace string) (protocol.ChangeEntry, bool) {
	path, ok := repoPathToWorkspace(entry.Path, repoRoot, workspace)
	if !ok {
		return protocol.ChangeEntry{}, false
	}
	entry.Path = path

	if entry.OldPath != "" {
		if oldPath, ok := repoPathToWorkspace(entry.OldPath, repoRoot, workspace); ok {
			entry.OldPath = oldPath
		} else {
			entry.OldPath = ""
		}
	}

	return entry, true
}

func repoPathToWorkspace(path string, repoRoot string, workspace string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}

	abs := filepath.Clean(filepath.Join(repoRoot, filepath.FromSlash(path)))
	if !insideRoot(workspace, abs) {
		return "", false
	}

	rel, err := filepath.Rel(workspace, abs)
	if err != nil {
		return "", false
	}
	if rel == "." {
		return ".", true
	}
	return filepath.ToSlash(rel), true
}

func insideRoot(root string, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

func shouldHidePath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		lower := strings.ToLower(part)
		if ignoredNames[lower] || sensitiveNames[lower] {
			return true
		}
	}
	return false
}

func isBinary(content []byte) bool {
	if len(content) == 0 {
		return false
	}
	if bytes.IndexByte(content, 0) >= 0 {
		return true
	}
	return !utf8.Valid(content)
}

type limitedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	return &limitedBuffer{limit: limit}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		_, _ = b.buffer.Write(p)
		return len(p), nil
	}

	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		b.truncated = true
		_, _ = b.buffer.Write(p[:remaining])
		return len(p), nil
	}

	_, _ = b.buffer.Write(p)
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	return b.buffer.String()
}

func (b *limitedBuffer) Truncated() bool {
	return b.truncated
}
