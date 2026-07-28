package opensandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	sdk "github.com/alibaba/OpenSandbox/sdks/sandbox/go"

	domainsandbox "myai/core/domain/sandbox"
	sandboxport "myai/core/port/sandbox"
)

type Workspace struct {
	client           sandboxClient
	commandTimeout   time.Duration
	maxDownloadBytes int64
}

var _ sandboxport.Workspace = (*Workspace)(nil)

func (workspace *Workspace) ID() string {
	if workspace == nil || workspace.client == nil {
		return ""
	}
	return workspace.client.ID()
}

func (workspace *Workspace) Run(ctx context.Context, request domainsandbox.CommandRequest) (domainsandbox.CommandResult, error) {
	if workspace == nil || workspace.client == nil {
		return domainsandbox.CommandResult{}, fmt.Errorf("OpenSandbox workspace is not initialized")
	}
	if err := request.Validate(); err != nil {
		return domainsandbox.CommandResult{}, err
	}
	timeout := request.Timeout
	if timeout <= 0 || workspace.commandTimeout > 0 && timeout > workspace.commandTimeout {
		timeout = workspace.commandTimeout
	}
	runCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	execution, err := workspace.client.RunCommandWithOpts(runCtx, sdk.RunCommandRequest{
		Command: strings.TrimSpace(request.Command),
		Cwd:     strings.TrimSpace(request.WorkDir),
		Timeout: timeout.Milliseconds(),
		Envs:    cloneMap(request.Env),
	}, nil)
	result := mapExecution(execution)
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		result.TimedOut = true
		result.ExitCode = -1
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("run OpenSandbox command: %w", err)
	}
	return result, nil
}

func (workspace *Workspace) CreateDirectory(ctx context.Context, directory string, mode uint32) error {
	if workspace == nil || workspace.client == nil {
		return fmt.Errorf("OpenSandbox workspace is not initialized")
	}
	directory = strings.TrimSpace(directory)
	if directory == "" || !strings.HasPrefix(directory, "/") || path.Clean(directory) != directory {
		return fmt.Errorf("invalid OpenSandbox directory %q", directory)
	}
	if mode == 0 {
		mode = 0o755
	}
	return workspace.client.CreateDirectory(ctx, directory, octalDigits(mode))
}

func (workspace *Workspace) UploadFiles(ctx context.Context, files []domainsandbox.File) error {
	if workspace == nil || workspace.client == nil {
		return fmt.Errorf("OpenSandbox workspace is not initialized")
	}
	if len(files) == 0 {
		return nil
	}
	directories := make(map[string]struct{})
	entries := make([]sdk.UploadFileEntry, 0, len(files))
	for index, file := range files {
		if err := file.Validate(); err != nil {
			return fmt.Errorf("validate OpenSandbox upload file %d: %w", index, err)
		}
		directories[path.Dir(file.Path)] = struct{}{}
		mode := file.Mode
		if mode == 0 {
			mode = 0o644
		}
		entries = append(entries, sdk.UploadFileEntry{
			File: bytes.NewReader(file.Content),
			Options: sdk.UploadFileOptions{
				FileName: path.Base(file.Path),
				Metadata: sdk.FileMetadata{Path: file.Path, Mode: octalDigits(mode)},
			},
		})
	}
	for _, directory := range sortedDirectories(directories) {
		if directory == "/" || directory == "." {
			continue
		}
		if err := workspace.client.CreateDirectory(ctx, directory, 755); err != nil {
			return fmt.Errorf("create OpenSandbox upload directory %q: %w", directory, err)
		}
	}
	if err := workspace.client.UploadFiles(ctx, entries); err != nil {
		return fmt.Errorf("upload files to OpenSandbox: %w", err)
	}
	return nil
}

func (workspace *Workspace) DownloadFile(ctx context.Context, remotePath string, maxBytes int64) ([]byte, error) {
	if workspace == nil || workspace.client == nil {
		return nil, fmt.Errorf("OpenSandbox workspace is not initialized")
	}
	remotePath = strings.TrimSpace(remotePath)
	if remotePath == "" || !strings.HasPrefix(remotePath, "/") || path.Clean(remotePath) != remotePath {
		return nil, fmt.Errorf("invalid OpenSandbox file path %q", remotePath)
	}
	if maxBytes <= 0 || workspace.maxDownloadBytes > 0 && maxBytes > workspace.maxDownloadBytes {
		maxBytes = workspace.maxDownloadBytes
	}
	reader, err := workspace.client.DownloadFile(ctx, remotePath, "")
	if err != nil {
		return nil, fmt.Errorf("download OpenSandbox file %q: %w", remotePath, err)
	}
	defer reader.Close()
	content, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read OpenSandbox file %q: %w", remotePath, err)
	}
	if int64(len(content)) > maxBytes {
		return nil, fmt.Errorf("OpenSandbox file %q exceeds download limit of %d bytes", remotePath, maxBytes)
	}
	return content, nil
}

func (workspace *Workspace) Destroy(ctx context.Context) error {
	if workspace == nil || workspace.client == nil {
		return nil
	}
	return workspace.client.Kill(ctx)
}

func (workspace *Workspace) Close() error {
	if workspace == nil || workspace.client == nil {
		return nil
	}
	return workspace.client.Close()
}

func mapExecution(execution *sdk.Execution) domainsandbox.CommandResult {
	result := domainsandbox.CommandResult{ExitCode: -1}
	if execution == nil {
		return result
	}
	result.ExecutionID = execution.ID
	result.Stdout = execution.Text()
	result.Stderr = joinOutput(execution.Stderr)
	if execution.ExitCode != nil {
		result.ExitCode = *execution.ExitCode
	}
	if execution.Error != nil {
		result.ErrorName = execution.Error.Name
		result.ErrorValue = execution.Error.Value
	}
	return result
}

func joinOutput(messages []sdk.OutputMessage) string {
	values := make([]string, 0, len(messages))
	for _, message := range messages {
		values = append(values, message.Text)
	}
	return strings.Join(values, "\n")
}

func sortedDirectories(values map[string]struct{}) []string {
	directories := make([]string, 0, len(values))
	for directory := range values {
		directories = append(directories, directory)
	}
	sort.Slice(directories, func(left, right int) bool {
		leftDepth := strings.Count(directories[left], "/")
		rightDepth := strings.Count(directories[right], "/")
		if leftDepth == rightDepth {
			return directories[left] < directories[right]
		}
		return leftDepth < rightDepth
	})
	return directories
}

func octalDigits(mode uint32) int {
	return int(mode/64%8)*100 + int(mode/8%8)*10 + int(mode%8)
}
