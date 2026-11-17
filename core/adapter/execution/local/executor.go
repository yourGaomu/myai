package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"unicode/utf8"

	domainexecution "myai/core/domain/execution"
	executionport "myai/core/port/execution"
	"myai/core/processpolicy"
)

const (
	defaultTimeout        = 30 * time.Second
	maxTimeout            = 2 * time.Minute
	defaultMaxOutputBytes = 64 * 1024
	maxOutputBytes        = 256 * 1024
)

// Executor runs on the host OS. Workspace path checks and the destructive
// command denylist are safeguards, not an OS or container isolation boundary.
type Executor struct {
	workspace string
}

var _ executionport.CommandExecutor = (*Executor)(nil)

func New(workspace string) (*Executor, error) {
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
	return &Executor{workspace: absWorkspace}, nil
}

func (executor *Executor) Run(ctx context.Context, request domainexecution.RunRequest) (domainexecution.RunResult, error) {
	command := strings.TrimSpace(request.Command)
	if command == "" {
		return domainexecution.RunResult{}, errors.New("command is empty")
	}
	if err := rejectDangerousCommand(command); err != nil {
		return domainexecution.RunResult{}, err
	}
	workDir, err := executor.cleanWorkDir(request.WorkDir)
	if err != nil {
		return domainexecution.RunResult{}, err
	}
	timeout := normalizeTimeout(request.Timeout)
	outputLimit := normalizeOutputLimit(request.MaxOutputBytes)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd, shellName := shellCommand(runCtx, command)
	cmd.Dir = workDir
	cmd.Env = processpolicy.Environment(nil)
	stdout := newLimitedBuffer(outputLimit)
	stderr := newLimitedBuffer(outputLimit)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	start := time.Now()
	err = processpolicy.Run(cmd)
	result := domainexecution.RunResult{
		Command: command, WorkDir: filepath.ToSlash(relativePath(executor.workspace, workDir)),
		ExitCode: 0, Stdout: stdout.String(), Stderr: stderr.String(),
		TimedOut: runCtx.Err() == context.DeadlineExceeded, Truncated: stdout.Truncated() || stderr.Truncated(),
		DurationMS: time.Since(start).Milliseconds(), ExecutionEnvironment: "local-host", Isolated: false, Shell: shellName,
	}
	if err == nil {
		return result, nil
	}
	if result.TimedOut {
		result.ExitCode = -1
		result.ErrorMessage = "command timed out"
		return result, nil
	}
	if runCtx.Err() != nil {
		result.ExitCode = -1
		result.ErrorMessage = "command canceled"
		return result, runCtx.Err()
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
		result.ErrorMessage = err.Error()
		return result, nil
	}
	return domainexecution.RunResult{}, err
}

func (executor *Executor) cleanWorkDir(workDir string) (string, error) {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return executor.workspace, nil
	}
	if !filepath.IsAbs(workDir) {
		workDir = filepath.Join(executor.workspace, workDir)
	}
	absWorkDir, err := filepath.Abs(filepath.Clean(workDir))
	if err != nil {
		return "", err
	}
	if !isInside(executor.workspace, absWorkDir) {
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
		// Windows PowerShell otherwise encodes redirected output using a legacy
		// code page. Set both the console and native-program pipe encoding.
		command = "$utf8 = New-Object System.Text.UTF8Encoding $false; [Console]::OutputEncoding = $utf8; $OutputEncoding = $utf8; " + command
		return exec.CommandContext(ctx, "powershell", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", command), "powershell"
	}
	return exec.CommandContext(ctx, "sh", "-c", command), "sh"
}

func rejectDangerousCommand(command string) error {
	normalized := strings.ToLower(strings.Join(strings.Fields(command), " "))
	blocked := []string{
		"rm -rf /", "rm -fr /", "remove-item -recurse", "remove-item -r", "del /s", "erase /s",
		"rd /s", "rmdir /s", "format ", "diskpart", "mkfs", "dd if=", "shutdown", "reboot",
		"halt", "poweroff", "bcdedit", "reg delete",
	}
	for _, pattern := range blocked {
		if strings.Contains(normalized, pattern) {
			return fmt.Errorf("command blocked by local executor policy: %s", pattern)
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
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
	head      []byte
	tail      []byte
	limit     int
	total     int64
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer { return &limitedBuffer{limit: max(0, limit)} }

func (buffer *limitedBuffer) Write(content []byte) (int, error) {
	written := len(content)
	buffer.total += int64(written)
	buffer.truncated = buffer.total > int64(buffer.limit)
	headLimit := (buffer.limit + 1) / 2
	if remaining := headLimit - len(buffer.head); remaining > 0 {
		count := min(remaining, len(content))
		buffer.head = append(buffer.head, content[:count]...)
		content = content[count:]
	}
	tailLimit := buffer.limit - headLimit
	if tailLimit > 0 && len(content) > 0 {
		if len(content) >= tailLimit {
			buffer.tail = append(buffer.tail[:0], content[len(content)-tailLimit:]...)
		} else {
			if overflow := len(buffer.tail) + len(content) - tailLimit; overflow > 0 {
				copy(buffer.tail, buffer.tail[overflow:])
				buffer.tail = buffer.tail[:len(buffer.tail)-overflow]
			}
			buffer.tail = append(buffer.tail, content...)
		}
	}
	return written, nil
}

func (buffer *limitedBuffer) String() string {
	if !buffer.truncated {
		return strings.ToValidUTF8(string(buffer.head)+string(buffer.tail), "?")
	}
	// Trim incomplete runes at the two cut boundaries. Invalid source encoding
	// is replaced one byte at a time so rendered output stays within the limit.
	head := buffer.head
	for len(head) > 0 {
		_, size := utf8.DecodeLastRune(head)
		if size != 1 || head[len(head)-1] < utf8.RuneSelf {
			break
		}
		head = head[:len(head)-1]
	}
	tail := buffer.tail
	for len(tail) > 0 && !utf8.RuneStart(tail[0]) {
		tail = tail[1:]
	}
	return strings.ToValidUTF8(string(head), "?") + strings.ToValidUTF8(string(tail), "?")
}

func (buffer *limitedBuffer) Truncated() bool { return buffer.truncated }
