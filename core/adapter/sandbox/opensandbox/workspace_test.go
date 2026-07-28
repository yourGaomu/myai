package opensandbox

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	sdk "github.com/alibaba/OpenSandbox/sdks/sandbox/go"

	domainsandbox "myai/core/domain/sandbox"
)

type fakeSandboxClient struct {
	command     sdk.RunCommandRequest
	directories []string
	uploads     []sdk.UploadFileEntry
	download    string
}

func (client *fakeSandboxClient) ID() string { return "sandbox-1" }

func (client *fakeSandboxClient) RunCommandWithOpts(_ context.Context, request sdk.RunCommandRequest, _ *sdk.ExecutionHandlers) (*sdk.Execution, error) {
	client.command = request
	exitCode := 0
	return &sdk.Execution{
		ID:       "execution-1",
		Stdout:   []sdk.OutputMessage{{Text: "ok"}},
		Stderr:   []sdk.OutputMessage{{Text: "warning"}},
		ExitCode: &exitCode,
	}, nil
}

func (client *fakeSandboxClient) CreateDirectory(_ context.Context, path string, _ int) error {
	client.directories = append(client.directories, path)
	return nil
}

func (client *fakeSandboxClient) UploadFiles(_ context.Context, entries []sdk.UploadFileEntry) error {
	client.uploads = entries
	return nil
}

func (client *fakeSandboxClient) DownloadFile(_ context.Context, _ string, _ string, _ ...sdk.DownloadFileOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(client.download)), nil
}

func (client *fakeSandboxClient) Kill(context.Context) error { return nil }
func (client *fakeSandboxClient) Close() error               { return nil }

func TestWorkspaceRunsAndUploadsInsideRemoteWorkspace(t *testing.T) {
	client := &fakeSandboxClient{}
	workspace := &Workspace{client: client, commandTimeout: time.Minute, maxDownloadBytes: 16}
	result, err := workspace.Run(context.Background(), domainsandbox.CommandRequest{
		Command: "python /audit/check.py",
		WorkDir: "/audit",
		Timeout: 2 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "ok" || result.Stderr != "warning" || result.ExitCode != 0 {
		t.Fatalf("unexpected command result: %#v", result)
	}
	if client.command.Timeout != time.Minute.Milliseconds() {
		t.Fatalf("expected command timeout to be capped, got %d", client.command.Timeout)
	}

	err = workspace.UploadFiles(context.Background(), []domainsandbox.File{
		{Path: "/audit/skill/SKILL.md", Content: []byte("# Skill")},
		{Path: "/audit/skill/scripts/check.py", Content: []byte("print('ok')"), Mode: 0o755},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(client.uploads) != 2 || len(client.directories) != 2 {
		t.Fatalf("unexpected upload calls: directories=%v uploads=%d", client.directories, len(client.uploads))
	}
}

func TestWorkspaceRejectsOversizedDownload(t *testing.T) {
	client := &fakeSandboxClient{download: "0123456789"}
	workspace := &Workspace{client: client, maxDownloadBytes: 8}
	if _, err := workspace.DownloadFile(context.Background(), "/audit/result.json", 0); err == nil {
		t.Fatal("expected oversized download to be rejected")
	}
}
