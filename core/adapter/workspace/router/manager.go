package router

import (
	"context"
	"errors"
	"fmt"

	domainsandbox "myai/core/domain/sandbox"
	domainworkspace "myai/core/domain/workspace"
	workspaceport "myai/core/port/workspace"
)

type Manager struct {
	Snapshot    workspaceport.Manager
	OpenSandbox workspaceport.Manager
	Remote      workspaceport.CommandRunner
}

func (manager *Manager) Close() error {
	if manager == nil {
		return nil
	}
	closer, ok := manager.OpenSandbox.(interface{ Close() error })
	if !ok || closer == nil {
		return nil
	}
	return closer.Close()
}

var _ workspaceport.Manager = (*Manager)(nil)
var _ workspaceport.CommandRunner = (*Manager)(nil)

func (manager *Manager) Prepare(ctx context.Context, request workspaceport.PrepareRequest) (workspaceport.PreparedWorkspace, error) {
	switch request.Mode {
	case domainworkspace.IsolationModeSnapshot:
		return manager.prepareWith(ctx, manager.Snapshot, request)
	case domainworkspace.IsolationModeGitWorktree:
		// Git is an optional optimization. Snapshot remains the safe fallback when no worktree adapter is configured.
		request.Mode = domainworkspace.IsolationModeSnapshot
		return manager.prepareWith(ctx, manager.Snapshot, request)
	case domainworkspace.IsolationModeOpenSandbox:
		return manager.prepareWith(ctx, manager.OpenSandbox, request)
	default:
		return workspaceport.PreparedWorkspace{}, fmt.Errorf("unsupported isolated workspace mode %q", request.Mode)
	}
}

func (manager *Manager) Collect(ctx context.Context, request workspaceport.CollectRequest) (workspaceport.CollectedChanges, error) {
	selected, err := manager.forReference(request.Reference)
	if err != nil {
		return workspaceport.CollectedChanges{}, err
	}
	return selected.Collect(ctx, request)
}

func (manager *Manager) Apply(ctx context.Context, request workspaceport.ApplyRequest) (workspaceport.AppliedChanges, error) {
	selected, err := manager.forReference(request.Reference)
	if err != nil {
		return workspaceport.AppliedChanges{}, err
	}
	return selected.Apply(ctx, request)
}

func (manager *Manager) Discard(ctx context.Context, request workspaceport.DiscardRequest) (workspaceport.DiscardedChanges, error) {
	selected, err := manager.forReference(request.Reference)
	if err != nil {
		return workspaceport.DiscardedChanges{}, err
	}
	return selected.Discard(ctx, request)
}

func (manager *Manager) Run(ctx context.Context, sandboxID string, localRoot string, request domainsandbox.CommandRequest) (domainsandbox.CommandResult, error) {
	if manager == nil || manager.Remote == nil {
		return domainsandbox.CommandResult{}, errors.New("remote workspace command runner is not configured")
	}
	return manager.Remote.Run(ctx, sandboxID, localRoot, request)
}

func (manager *Manager) prepareWith(ctx context.Context, selected workspaceport.Manager, request workspaceport.PrepareRequest) (workspaceport.PreparedWorkspace, error) {
	if selected == nil {
		return workspaceport.PreparedWorkspace{}, fmt.Errorf("workspace isolation mode %q is not configured", request.Mode)
	}
	return selected.Prepare(ctx, request)
}

func (manager *Manager) forReference(reference domainworkspace.Reference) (workspaceport.Manager, error) {
	if manager == nil {
		return nil, errors.New("workspace manager router is nil")
	}
	switch reference.Mode {
	case domainworkspace.IsolationModeSnapshot:
		if manager.Snapshot != nil {
			return manager.Snapshot, nil
		}
	case domainworkspace.IsolationModeOpenSandbox:
		if manager.OpenSandbox != nil {
			return manager.OpenSandbox, nil
		}
	}
	return nil, fmt.Errorf("workspace isolation mode %q is not configured", reference.Mode)
}
