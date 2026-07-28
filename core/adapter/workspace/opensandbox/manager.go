package opensandbox

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	domainsandbox "myai/core/domain/sandbox"
	domainworkspace "myai/core/domain/workspace"
	sandboxport "myai/core/port/sandbox"
	workspaceport "myai/core/port/workspace"
)

type remoteWorkspace interface {
	ID() string
	Run(ctx context.Context, request domainsandbox.CommandRequest) (domainsandbox.CommandResult, error)
	CreateDirectory(ctx context.Context, path string, mode uint32) error
	UploadFiles(ctx context.Context, files []domainsandbox.File) error
	DownloadFile(ctx context.Context, path string, maxBytes int64) ([]byte, error)
	Destroy(ctx context.Context) error
	Close() error
}

type workspaceState struct {
	mu     sync.Mutex
	local  domainworkspace.Reference
	remote remoteWorkspace
	synced map[string]fileState
}

type Manager struct {
	local  workspaceport.Manager
	remote sandboxport.Manager
	mu     sync.Mutex
	states map[string]*workspaceState
}

var _ workspaceport.Manager = (*Manager)(nil)
var _ workspaceport.CommandRunner = (*Manager)(nil)

func New(local workspaceport.Manager, remote sandboxport.Manager) (*Manager, error) {
	if local == nil {
		return nil, errors.New("OpenSandbox local snapshot manager is nil")
	}
	if remote == nil {
		return nil, errors.New("OpenSandbox remote manager is nil")
	}
	return &Manager{local: local, remote: remote, states: make(map[string]*workspaceState)}, nil
}

func (manager *Manager) Prepare(ctx context.Context, request workspaceport.PrepareRequest) (workspaceport.PreparedWorkspace, error) {
	if request.Mode != domainworkspace.IsolationModeOpenSandbox {
		return workspaceport.PreparedWorkspace{}, fmt.Errorf("OpenSandbox manager does not support isolation mode %q", request.Mode)
	}
	localRequest := request
	localRequest.Mode = domainworkspace.IsolationModeSnapshot
	prepared, err := manager.local.Prepare(ctx, localRequest)
	if err != nil {
		return workspaceport.PreparedWorkspace{}, err
	}
	remote, err := manager.remote.Create(ctx, domainsandbox.CreateRequest{
		Metadata:      map[string]string{"task_id": request.TaskID, "session_id": request.SessionID, "workspace_id": request.WorkspaceID},
		NetworkPolicy: &domainsandbox.NetworkPolicy{DefaultAction: domainsandbox.NetworkActionDeny},
	})
	if err != nil {
		_, _ = manager.local.Discard(context.Background(), workspaceport.DiscardRequest{Reference: prepared.Reference})
		return workspaceport.PreparedWorkspace{}, err
	}
	cleanup := func() {
		_ = remote.Destroy(context.Background())
		_ = remote.Close()
		_, _ = manager.local.Discard(context.Background(), workspaceport.DiscardRequest{Reference: prepared.Reference})
	}
	if err := remote.CreateDirectory(ctx, "/workspace", 0o755); err != nil {
		cleanup()
		return workspaceport.PreparedWorkspace{}, err
	}
	files, err := scanLocal(ctx, prepared.Reference.Root)
	if err != nil {
		cleanup()
		return workspaceport.PreparedWorkspace{}, err
	}
	if err := uploadChanged(ctx, remote, prepared.Reference.Root, nil, files); err != nil {
		cleanup()
		return workspaceport.PreparedWorkspace{}, err
	}
	state := &workspaceState{local: prepared.Reference, remote: remote, synced: files}
	manager.mu.Lock()
	manager.states[remote.ID()] = state
	manager.mu.Unlock()
	return workspaceport.PreparedWorkspace{Reference: domainworkspace.Reference{
		ID: request.WorkspaceID, Mode: domainworkspace.IsolationModeOpenSandbox,
		Root: prepared.Reference.Root, SandboxID: remote.ID(),
	}}, nil
}

func (manager *Manager) Collect(ctx context.Context, request workspaceport.CollectRequest) (workspaceport.CollectedChanges, error) {
	return manager.local.Collect(ctx, workspaceport.CollectRequest{Reference: localReference(request.Reference)})
}

func (manager *Manager) Apply(ctx context.Context, request workspaceport.ApplyRequest) (workspaceport.AppliedChanges, error) {
	sandboxID := request.Reference.SandboxID
	request.Reference = localReference(request.Reference)
	result, err := manager.local.Apply(ctx, request)
	if err != nil {
		return result, err
	}
	manager.release(ctx, sandboxID)
	return result, nil
}

func (manager *Manager) Discard(ctx context.Context, request workspaceport.DiscardRequest) (workspaceport.DiscardedChanges, error) {
	manager.release(ctx, request.Reference.SandboxID)
	request.Reference = localReference(request.Reference)
	return manager.local.Discard(ctx, request)
}

func (manager *Manager) release(ctx context.Context, sandboxID string) {
	manager.mu.Lock()
	state := manager.states[sandboxID]
	delete(manager.states, sandboxID)
	manager.mu.Unlock()
	if state == nil {
		return
	}
	state.mu.Lock()
	_ = state.remote.Destroy(ctx)
	_ = state.remote.Close()
	state.mu.Unlock()
}

func (manager *Manager) Close() error {
	if manager == nil {
		return nil
	}
	manager.mu.Lock()
	states := manager.states
	manager.states = make(map[string]*workspaceState)
	manager.mu.Unlock()
	var errs []error
	for _, state := range states {
		state.mu.Lock()
		errs = append(errs, state.remote.Destroy(context.Background()), state.remote.Close())
		state.mu.Unlock()
	}
	return errors.Join(errs...)
}

func (manager *Manager) Run(ctx context.Context, sandboxID string, localRoot string, request domainsandbox.CommandRequest) (domainsandbox.CommandResult, error) {
	state, err := manager.state(ctx, sandboxID, localRoot)
	if err != nil {
		return domainsandbox.CommandResult{}, err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	current, err := scanLocal(ctx, localRoot)
	if err != nil {
		return domainsandbox.CommandResult{}, err
	}
	if err := uploadChanged(ctx, state.remote, localRoot, state.synced, current); err != nil {
		return domainsandbox.CommandResult{}, err
	}
	state.synced = current
	workDir, err := remoteWorkDir(request.WorkDir)
	if err != nil {
		return domainsandbox.CommandResult{}, err
	}
	request.WorkDir = workDir
	result, runErr := state.remote.Run(ctx, request)
	if syncErr := synchronizeDown(ctx, state.remote, localRoot, state.synced); syncErr != nil {
		return result, errors.Join(runErr, syncErr)
	}
	state.synced, err = scanLocal(ctx, localRoot)
	if err != nil {
		return result, errors.Join(runErr, err)
	}
	return result, runErr
}

func (manager *Manager) state(ctx context.Context, sandboxID string, localRoot string) (*workspaceState, error) {
	sandboxID = strings.TrimSpace(sandboxID)
	if sandboxID == "" {
		return nil, errors.New("OpenSandbox id is empty")
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if state := manager.states[sandboxID]; state != nil {
		if filepath.Clean(state.local.Root) != filepath.Clean(localRoot) {
			return nil, errors.New("OpenSandbox local mirror does not match the session workspace")
		}
		return state, nil
	}
	remote, err := manager.remote.Connect(ctx, sandboxID)
	if err != nil {
		return nil, err
	}
	files, err := scanLocal(ctx, localRoot)
	if err != nil {
		_ = remote.Close()
		return nil, err
	}
	state := &workspaceState{
		local:  domainworkspace.Reference{Root: localRoot, Mode: domainworkspace.IsolationModeSnapshot},
		remote: remote, synced: files,
	}
	manager.states[sandboxID] = state
	return state, nil
}

func localReference(reference domainworkspace.Reference) domainworkspace.Reference {
	reference.Mode = domainworkspace.IsolationModeSnapshot
	reference.SandboxID = ""
	return reference
}

func remoteWorkDir(value string) (string, error) {
	value = filepath.ToSlash(strings.TrimSpace(value))
	if value == "" || value == "." {
		return "/workspace", nil
	}
	if strings.HasPrefix(value, "/") || value == ".." || strings.HasPrefix(value, "../") || strings.Contains(value, "/../") {
		return "", fmt.Errorf("work_dir escapes OpenSandbox workspace: %s", value)
	}
	return "/workspace/" + value, nil
}
