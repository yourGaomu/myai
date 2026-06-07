package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	defaultTimeout        = 30 * time.Second
	maxTimeout            = 2 * time.Minute
	defaultMaxOutputBytes = 64 * 1024
	maxOutputBytes        = 256 * 1024
)

type RunRequest struct {
	Command        string
	WorkDir        string
	Timeout        time.Duration
	MaxOutputBytes int
}

type RunResult struct {
	Command      string `json:"command"`
	WorkDir      string `json:"work_dir"`
	ExitCode     int    `json:"exit_code"`
	Stdout       string `json:"stdout"`
	Stderr       string `json:"stderr"`
	TimedOut     bool   `json:"timed_out"`
	Truncated    bool   `json:"truncated"`
	DurationMS   int64  `json:"duration_ms"`
	Sandbox      string `json:"sandbox"`
	Shell        string `json:"shell"`
	ErrorMessage string `json:"error,omitempty"`
}

type Sandbox interface {
	Run(ctx context.Context, request RunRequest) (RunResult, error)
}

type LocalSandbox struct {
	workspace string
}

func NewLocalSandbox(workspace string) (*LocalSandbox, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		current, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		workspace = current
	}

	absWorkspace, err := filepath.Abs(filepath.Clean(workspace))
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(absWorkspace)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace is not a directory: %s", absWorkspace)
	}

	return &LocalSandbox{workspace: absWorkspace}, nil
}

func (s *LocalSandbox) Run(ctx context.Context, request RunRequest) (RunResult, error) {
	command := strings.TrimSpace(request.Command)
	if command == "" {
		return RunResult{}, errors.New("command is empty")
	}
	if err := rejectDangerousCommand(command); err != nil {
		return RunResult{}, err
	}

	workDir, err := s.cleanWorkDir(request.WorkDir)
	if err != nil {
		return RunResult{}, err
	}

	timeout := normalizeTimeout(request.Timeout)
	outputLimit := normalizeOutputLimit(request.MaxOutputBytes)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd, shellName := shellCommand(runCtx, command)
	cmd.Dir = workDir

	stdout := newLimitedBuffer(outputLimit)
	stderr := newLimitedBuffer(outputLimit)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start)

	result := RunResult{
		Command:    command,
		WorkDir:    filepath.ToSlash(relativePath(s.workspace, workDir)),
		ExitCode:   0,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		TimedOut:   runCtx.Err() == context.DeadlineExceeded,
		Truncated:  stdout.Truncated() || stderr.Truncated(),
		DurationMS: duration.Milliseconds(),
		Sandbox:    "local",
		Shell:      shellName,
	}

	if err == nil {
		return result, nil
	}

	if result.TimedOut {
		result.ExitCode = -1
		result.ErrorMessage = "command timed out"
		return result, nil
	}

	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
		result.ErrorMessage = err.Error()
		return result, nil
	}

	return RunResult{}, err
}

func (s *LocalSandbox) cleanWorkDir(workDir string) (string, error) {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return s.workspace, nil
	}

	if !filepath.IsAbs(workDir) {
		workDir = filepath.Join(s.workspace, workDir)
	}

	absWorkDir, err := filepath.Abs(filepath.Clean(workDir))
	if err != nil {
		return "", err
	}
	if !isInside(s.workspace, absWorkDir) {
		return "", fmt.Errorf("work_dir is outside workspace: %s", workDir)
	}

	info, err := os.Stat(absWorkDir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("work_dir is not a directory: %s", workDir)
	}

	return absWorkDir, nil
}

func shellCommand(ctx context.Context, command string) (*exec.Cmd, string) {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(
			ctx,
			"powershell",
			"-NoLogo",
			"-NoProfile",
			"-NonInteractive",
			"-ExecutionPolicy",
			"Bypass",
			"-Command",
			command,
		), "powershell"
	}

	return exec.CommandContext(ctx, "sh", "-c", command), "sh"
}

func rejectDangerousCommand(command string) error {
	normalized := strings.ToLower(strings.Join(strings.Fields(command), " "))
	blocked := []string{
		"rm -rf /",
		"rm -fr /",
		"remove-item -recurse",
		"remove-item -r",
		"del /s",
		"erase /s",
		"rd /s",
		"rmdir /s",
		"format ",
		"diskpart",
		"mkfs",
		"dd if=",
		"shutdown",
		"reboot",
		"halt",
		"poweroff",
		"bcdedit",
		"reg delete",
	}

	for _, pattern := range blocked {
		if strings.Contains(normalized, pattern) {
			return fmt.Errorf("command blocked by local sandbox policy: %s", pattern)
		}
	}

	return nil
}

func normalizeTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return defaultTimeout
	}
	if timeout > maxTimeout {
		return maxTimeout
	}
	return timeout
}

func normalizeOutputLimit(limit int) int {
	if limit <= 0 {
		return defaultMaxOutputBytes
	}
	if limit > maxOutputBytes {
		return maxOutputBytes
	}
	return limit
}

func isInside(root string, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func relativePath(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	if rel == "." {
		return "."
	}
	return rel
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
		b.truncated = true
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
