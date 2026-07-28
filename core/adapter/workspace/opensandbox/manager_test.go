package opensandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainsandbox "myai/core/domain/sandbox"
	domainworkspace "myai/core/domain/workspace"
	sandboxport "myai/core/port/sandbox"
	workspaceport "myai/core/port/workspace"
)

func TestManagerPrepareUploadsSnapshotOnce(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "main.go", "before\n")
	local := &fakeLocalManager{root: root}
	remote := newFakeRemoteManager()
	manager, err := New(local, remote)
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := manager.Prepare(context.Background(), workspaceport.PrepareRequest{
		WorkspaceID: "workspace-1", Mode: domainworkspace.IsolationModeOpenSandbox,
		SourceRoot: "C:/source", TaskID: "task-1", SessionID: "session-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := remote.created[0]
	if prepared.Reference.Mode != domainworkspace.IsolationModeOpenSandbox || prepared.Reference.SandboxID != workspace.id {
		t.Fatalf("unexpected prepared workspace: %#v", prepared.Reference)
	}
	if len(workspace.uploads) != 1 || len(workspace.uploads[0]) != 1 {
		t.Fatalf("expected one initial upload batch, got %#v", workspace.uploads)
	}
	if got := string(workspace.files["/workspace/main.go"].Content); got != "before\n" {
		t.Fatalf("unexpected remote content %q", got)
	}
	if workspace.createDirectoryCalls != 1 {
		t.Fatalf("expected workspace directory to be created once, got %d", workspace.createDirectoryCalls)
	}
}

func TestManagerRunUploadsIncrementalChangesAndSynchronizesRemoteChanges(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "main.go", "before\n")
	writeWorkspaceFile(t, root, "delete.txt", "remove me\n")
	local := &fakeLocalManager{root: root}
	remote := newFakeRemoteManager()
	manager, err := New(local, remote)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := manager.Prepare(context.Background(), workspaceport.PrepareRequest{
		WorkspaceID: "workspace-2", Mode: domainworkspace.IsolationModeOpenSandbox,
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := remote.created[0]
	writeWorkspaceFile(t, root, "main.go", "local edit\n")
	workspace.onCommand = func() {
		workspace.files["/workspace/main.go"] = domainsandbox.File{Path: "/workspace/main.go", Content: []byte("remote edit\n"), Mode: 0o644}
		workspace.files["/workspace/new.txt"] = domainsandbox.File{Path: "/workspace/new.txt", Content: []byte("created remotely\n"), Mode: 0o644}
		delete(workspace.files, "/workspace/delete.txt")
	}

	result, err := manager.Run(context.Background(), prepared.Reference.SandboxID, root, domainsandbox.CommandRequest{
		Command: "mutate", WorkDir: "subdir",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || workspace.lastCommandWorkDir != "/workspace/subdir" {
		t.Fatalf("unexpected command result=%#v workdir=%q", result, workspace.lastCommandWorkDir)
	}
	if len(workspace.uploads) != 2 || len(workspace.uploads[1]) != 1 || workspace.uploads[1][0].Path != "/workspace/main.go" {
		t.Fatalf("expected only the local edit in the incremental upload, got %#v", workspace.uploads)
	}
	assertWorkspaceFile(t, root, "main.go", "remote edit\n")
	assertWorkspaceFile(t, root, "new.txt", "created remotely\n")
	if _, err := os.Stat(filepath.Join(root, "delete.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected remotely deleted file to be removed locally, err=%v", err)
	}
}

func TestManagerDiscardAndCloseDestroyRemoteWorkspaces(t *testing.T) {
	firstRoot := t.TempDir()
	writeWorkspaceFile(t, firstRoot, "first.txt", "first")
	firstLocal := &fakeLocalManager{root: firstRoot}
	remote := newFakeRemoteManager()
	manager, err := New(firstLocal, remote)
	if err != nil {
		t.Fatal(err)
	}
	first, err := manager.Prepare(context.Background(), workspaceport.PrepareRequest{
		WorkspaceID: "workspace-3", Mode: domainworkspace.IsolationModeOpenSandbox,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Discard(context.Background(), workspaceport.DiscardRequest{Reference: first.Reference}); err != nil {
		t.Fatal(err)
	}
	if !remote.created[0].destroyed || !remote.created[0].closed || firstLocal.discardCalls != 1 {
		t.Fatalf("discard did not release both workspace layers: remote=%#v local_discards=%d", remote.created[0], firstLocal.discardCalls)
	}

	secondRoot := t.TempDir()
	writeWorkspaceFile(t, secondRoot, "second.txt", "second")
	manager.local = &fakeLocalManager{root: secondRoot}
	if _, err := manager.Prepare(context.Background(), workspaceport.PrepareRequest{
		WorkspaceID: "workspace-4", Mode: domainworkspace.IsolationModeOpenSandbox,
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	if !remote.created[1].destroyed || !remote.created[1].closed {
		t.Fatalf("close did not destroy remaining remote workspace: %#v", remote.created[1])
	}
}

func TestWorkspaceSynchronizationRejectsEscapingAndOversizedFiles(t *testing.T) {
	root := t.TempDir()
	if _, err := safeLocalPath(root, "../outside.txt"); err == nil {
		t.Fatal("expected escaping path to be rejected")
	}
	workspace := &fakeRemoteWorkspace{id: "sandbox-1", files: make(map[string]domainsandbox.File)}
	err := uploadChanged(context.Background(), workspace, root, nil, map[string]fileState{
		"large.bin": {Path: "large.bin", Size: maxSynchronizedFileBytes + 1},
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds OpenSandbox synchronization limit") {
		t.Fatalf("expected oversized file error, got %v", err)
	}
	if len(workspace.uploads) != 0 {
		t.Fatalf("oversized file must not be uploaded, got %#v", workspace.uploads)
	}
}

type fakeLocalManager struct {
	root         string
	discardCalls int
}

func (manager *fakeLocalManager) Prepare(_ context.Context, request workspaceport.PrepareRequest) (workspaceport.PreparedWorkspace, error) {
	return workspaceport.PreparedWorkspace{Reference: domainworkspace.Reference{
		ID: request.WorkspaceID, Mode: domainworkspace.IsolationModeSnapshot, Root: manager.root,
	}}, nil
}

func (*fakeLocalManager) Collect(_ context.Context, request workspaceport.CollectRequest) (workspaceport.CollectedChanges, error) {
	return workspaceport.CollectedChanges{ChangeSet: domainworkspace.ChangeSet{WorkspaceID: request.Reference.ID}}, nil
}

func (*fakeLocalManager) Apply(_ context.Context, request workspaceport.ApplyRequest) (workspaceport.AppliedChanges, error) {
	return workspaceport.AppliedChanges{ChangeSet: domainworkspace.ChangeSet{
		WorkspaceID: request.Reference.ID, Status: domainworkspace.ChangeSetStatusApplied,
	}}, nil
}

func (manager *fakeLocalManager) Discard(_ context.Context, request workspaceport.DiscardRequest) (workspaceport.DiscardedChanges, error) {
	manager.discardCalls++
	return workspaceport.DiscardedChanges{ChangeSet: domainworkspace.ChangeSet{
		WorkspaceID: request.Reference.ID, Status: domainworkspace.ChangeSetStatusDiscarded,
	}}, nil
}

type fakeRemoteManager struct {
	created []*fakeRemoteWorkspace
}

func newFakeRemoteManager() *fakeRemoteManager {
	return &fakeRemoteManager{}
}

func (manager *fakeRemoteManager) Create(context.Context, domainsandbox.CreateRequest) (sandboxport.Workspace, error) {
	workspace := &fakeRemoteWorkspace{
		id: "sandbox-" + string(rune('1'+len(manager.created))), files: make(map[string]domainsandbox.File),
	}
	manager.created = append(manager.created, workspace)
	return workspace, nil
}

func (manager *fakeRemoteManager) Connect(_ context.Context, sandboxID string) (sandboxport.Workspace, error) {
	for _, workspace := range manager.created {
		if workspace.id == sandboxID {
			return workspace, nil
		}
	}
	return nil, errors.New("sandbox not found")
}

func (*fakeRemoteManager) Delete(context.Context, string) error { return nil }

type fakeRemoteWorkspace struct {
	id                   string
	files                map[string]domainsandbox.File
	uploads              [][]domainsandbox.File
	createDirectoryCalls int
	destroyed            bool
	closed               bool
	onCommand            func()
	lastCommandWorkDir   string
}

func (workspace *fakeRemoteWorkspace) ID() string { return workspace.id }

func (workspace *fakeRemoteWorkspace) Run(_ context.Context, request domainsandbox.CommandRequest) (domainsandbox.CommandResult, error) {
	if strings.HasPrefix(request.Command, "python -c") {
		manifest := make([]fileState, 0, len(workspace.files))
		for remotePath, file := range workspace.files {
			if !strings.HasPrefix(remotePath, "/workspace/") {
				continue
			}
			hash := sha256.Sum256(file.Content)
			manifest = append(manifest, fileState{
				Path: strings.TrimPrefix(remotePath, "/workspace/"), Hash: hex.EncodeToString(hash[:]),
				Size: int64(len(file.Content)), Mode: file.Mode,
			})
		}
		content, err := json.Marshal(manifest)
		if err != nil {
			return domainsandbox.CommandResult{}, err
		}
		workspace.files[remoteManifestPath] = domainsandbox.File{Path: remoteManifestPath, Content: content, Mode: 0o600}
		return domainsandbox.CommandResult{ExitCode: 0}, nil
	}
	workspace.lastCommandWorkDir = request.WorkDir
	if workspace.onCommand != nil {
		workspace.onCommand()
	}
	return domainsandbox.CommandResult{ExitCode: 0, Stdout: "done"}, nil
}

func (workspace *fakeRemoteWorkspace) CreateDirectory(context.Context, string, uint32) error {
	workspace.createDirectoryCalls++
	return nil
}

func (workspace *fakeRemoteWorkspace) UploadFiles(_ context.Context, files []domainsandbox.File) error {
	batch := make([]domainsandbox.File, len(files))
	for index, file := range files {
		batch[index] = domainsandbox.File{Path: file.Path, Content: append([]byte(nil), file.Content...), Mode: file.Mode}
		workspace.files[file.Path] = batch[index]
	}
	workspace.uploads = append(workspace.uploads, batch)
	return nil
}

func (workspace *fakeRemoteWorkspace) DownloadFile(_ context.Context, path string, maxBytes int64) ([]byte, error) {
	file, ok := workspace.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	if int64(len(file.Content)) > maxBytes {
		return nil, errors.New("download limit exceeded")
	}
	return append([]byte(nil), file.Content...), nil
}

func (workspace *fakeRemoteWorkspace) Destroy(context.Context) error {
	workspace.destroyed = true
	return nil
}

func (workspace *fakeRemoteWorkspace) Close() error {
	workspace.closed = true
	return nil
}

func writeWorkspaceFile(t *testing.T, root string, relative string, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertWorkspaceFile(t *testing.T, root string, relative string, expected string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != expected {
		t.Fatalf("expected %q, got %q", expected, string(content))
	}
}
