package session

import (
	"context"
	"errors"
	"strings"

	sessioncommand "myai/core/application/session/command"
	persistenceapi "myai/core/application/session/persistence/api"
	domainsubagent "myai/core/domain/subagent"
	repository "myai/core/port/repository"
	subagentport "myai/core/port/subagent"
	domainsession "myai/core/session"
	tooldef "myai/core/tool/tool"
)

type memoryStore interface {
	PutSessionState(state domainsession.InitialState, setCurrent bool) error
	GetSession(sessionID string) (*domainsession.Session, error)
	RemoveSession(sessionID string) error
	SetPermissionModeForSession(sessionID string, mode domainsession.PermissionMode) error
	SetPermissionCeilingForSession(sessionID string, ceiling []tooldef.Permission) error
	SetToolAllowlistForSession(sessionID string, allowed []string, enforce bool) error
	SetWorkspaceForSession(sessionID, root, sandbox string) error
}

type sessionLoader interface {
	Load(ctx context.Context, sessionID string) (*domainsession.Session, error)
}

type Factory struct {
	Memory      memoryStore
	Persistence persistenceapi.Service
	Loader      sessionLoader
}

var _ subagentport.ChildSessionFactory = Factory{}
var _ subagentport.ChildSessionLifecycle = Factory{}

func (factory Factory) Create(ctx context.Context, request subagentport.ChildSessionRequest) (*domainsession.Session, error) {
	if factory.Memory == nil {
		return nil, errors.New("subagent session memory is nil")
	}
	mode, ceiling, allowedTools, err := factory.childPermissions(ctx, request)
	if err != nil {
		return nil, err
	}
	if current, err := factory.Memory.GetSession(request.SessionID); err == nil {
		return factory.syncWorkspace(ctx, current, request, mode, ceiling, allowedTools)
	}
	if factory.Loader != nil {
		current, err := factory.Loader.Load(ctx, request.SessionID)
		if err == nil {
			return factory.syncWorkspace(ctx, current, request, mode, ceiling, allowedTools)
		}
		if !errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}
	}
	modelID := strings.TrimSpace(request.Definition.ModelID)
	if modelID == "" {
		modelID = strings.TrimSpace(request.FallbackModelID)
	}
	if modelID == "" {
		return nil, errors.New("subagent model id is required")
	}
	state := domainsession.InitialState{
		ID: request.SessionID, Kind: domainsession.KindSubagent,
		ParentSessionID: request.ParentSessionID, ParentTaskID: request.ParentTaskID,
		AgentDefinitionID: request.Definition.ID, AgentDefinitionVer: request.Definition.Version,
		SystemInstruction:    request.Definition.SystemPrompt,
		AllowedTools:         allowedTools,
		EnforceToolAllowlist: true,
		WorkspaceRoot:        request.WorkspaceRoot,
		WorkspaceSandboxID:   request.WorkspaceSandboxID,
		Model:                modelID,
		MaxToolRounds:        request.Definition.MaxTurns,
		AgentMode:            domainsession.AgentModeChat, PermissionMode: mode,
		PermissionCeiling: tooldef.ClonePermissionCeiling(ceiling),
		RAGSettings:       domainsession.RAGSettings{Mode: domainsession.RetrievalModeOff},
	}
	if err := factory.Memory.PutSessionState(state, false); err != nil {
		return nil, err
	}
	if factory.Persistence != nil {
		err := factory.Persistence.SaveRecord(ctx, repository.SessionRecord{
			ID: state.ID, Kind: string(state.Kind), ParentSessionID: state.ParentSessionID,
			ParentTaskID: state.ParentTaskID, AgentDefinitionID: state.AgentDefinitionID,
			AgentDefinitionVer: state.AgentDefinitionVer, SystemInstruction: state.SystemInstruction,
			AllowedTools: append([]string(nil), state.AllowedTools...), WorkspaceRoot: state.WorkspaceRoot,
			EnforceToolAllowlist: state.EnforceToolAllowlist,
			PermissionCeiling:    tooldef.ClonePermissionCeiling(state.PermissionCeiling),
			WorkspaceSandboxID:   state.WorkspaceSandboxID, MaxToolRounds: state.MaxToolRounds,
			Model: state.Model, AgentMode: string(state.AgentMode), PermissionMode: string(state.PermissionMode),
			Title: "Subagent: " + request.Definition.Name, RAGSettings: state.RAGSettings,
		})
		if err != nil {
			_ = factory.Memory.RemoveSession(state.ID)
			return nil, err
		}
	}
	return factory.Memory.GetSession(state.ID)
}

// Unload persists the current child session and removes only its live memory
// object. The durable session record remains available for a later follow-up
// or resume operation through Create.
func (factory Factory) Unload(ctx context.Context, sessionID string) error {
	if factory.Memory == nil {
		return errors.New("subagent session memory is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("subagent session id is empty")
	}
	current, err := factory.Memory.GetSession(sessionID)
	if err != nil {
		// RemoveSession is idempotent for the concrete store, so an already
		// unloaded session is not an error for lifecycle callers.
		return nil
	}
	if factory.Persistence != nil {
		if err := factory.Persistence.Save(ctx, sessioncommand.SaveSession{SessionID: sessionID, Model: current.Model}); err != nil {
			return err
		}
	}
	return factory.Memory.RemoveSession(sessionID)
}

func capabilityPermissionMode(mode domainsubagent.CapabilityMode) domainsession.PermissionMode {
	switch mode {
	case domainsubagent.CapabilityModeReadWrite:
		return domainsession.PermissionModeReadWrite
	case domainsubagent.CapabilityModeExecute:
		return domainsession.PermissionModeExecute
	case domainsubagent.CapabilityModeAll:
		return domainsession.PermissionModeFull
	default:
		return domainsession.PermissionModeReadonly
	}
}

func (factory Factory) childPermissions(ctx context.Context, request subagentport.ChildSessionRequest) (domainsession.PermissionMode, []tooldef.Permission, []string, error) {
	parentID := strings.TrimSpace(request.ParentSessionID)
	if parentID == "" || parentID == request.SessionID {
		return "", nil, nil, errors.New("subagent parent session is required")
	}
	parent, err := factory.Memory.GetSession(parentID)
	if err != nil && factory.Loader != nil {
		parent, err = factory.Loader.Load(ctx, parentID)
	}
	if err != nil || parent == nil {
		return "", nil, nil, errors.New("subagent parent session is unavailable")
	}
	mode := capabilityPermissionMode(request.Definition.CapabilityMode)
	ceiling := intersectPermissions(modePermissions(mode), modePermissions(parent.PermissionMode), parent.PermissionCeiling)
	if domainsession.NormalizeAgentMode(parent.AgentMode) == domainsession.AgentModePlan {
		ceiling = intersectPermissions(ceiling, []tooldef.Permission{tooldef.PermissionRead})
	}
	allowed := append([]string{}, request.Definition.AllowedTools...)
	if parent.EnforceToolAllowlist || parent.AllowedTools != nil {
		allowed = intersectTools(allowed, parent.AllowedTools)
	}
	if domainsession.NormalizePermissionMode(parent.PermissionMode) == domainsession.PermissionModeAsk {
		mode = domainsession.PermissionModeAsk
	} else {
		mode = modeForPermissions(ceiling)
	}
	return mode, ceiling, allowed, nil
}

func modePermissions(mode domainsession.PermissionMode) []tooldef.Permission {
	switch domainsession.NormalizePermissionMode(mode) {
	case domainsession.PermissionModeReadonly:
		return []tooldef.Permission{tooldef.PermissionRead}
	case domainsession.PermissionModeReadWrite:
		return []tooldef.Permission{tooldef.PermissionRead, tooldef.PermissionWrite}
	case domainsession.PermissionModeExecute:
		return []tooldef.Permission{tooldef.PermissionRead, tooldef.PermissionExecute}
	default:
		return []tooldef.Permission{tooldef.PermissionRead, tooldef.PermissionWrite, tooldef.PermissionExecute}
	}
}

func intersectPermissions(sets ...[]tooldef.Permission) []tooldef.Permission {
	result := make([]tooldef.Permission, 0, 3)
	for _, permission := range []tooldef.Permission{tooldef.PermissionRead, tooldef.PermissionWrite, tooldef.PermissionExecute} {
		allowed := true
		for _, set := range sets {
			allowed = allowed && tooldef.PermissionAllowsCeiling(permission, set)
		}
		if allowed {
			result = append(result, permission)
		}
	}
	return result
}

func modeForPermissions(permissions []tooldef.Permission) domainsession.PermissionMode {
	write := tooldef.PermissionAllowsCeiling(tooldef.PermissionWrite, permissions)
	execute := tooldef.PermissionAllowsCeiling(tooldef.PermissionExecute, permissions)
	switch {
	case write && execute:
		return domainsession.PermissionModeFull
	case write:
		return domainsession.PermissionModeReadWrite
	case execute:
		return domainsession.PermissionModeExecute
	default:
		return domainsession.PermissionModeReadonly
	}
}

func intersectTools(tools, ceiling []string) []string {
	result := make([]string, 0, len(tools))
	for _, name := range tools {
		for _, allowed := range ceiling {
			if name == allowed {
				result = append(result, name)
				break
			}
		}
	}
	return result
}

func (factory Factory) syncWorkspace(ctx context.Context, current *domainsession.Session, request subagentport.ChildSessionRequest, mode domainsession.PermissionMode, ceiling []tooldef.Permission, allowed []string) (*domainsession.Session, error) {
	if current == nil {
		return nil, errors.New("subagent child session is nil")
	}
	if current.ParentSessionID != "" && current.ParentSessionID != request.ParentSessionID {
		return nil, errors.New("subagent parent session does not match")
	}
	ceiling = intersectPermissions(ceiling, modePermissions(current.PermissionMode), current.PermissionCeiling)
	if domainsession.NormalizePermissionMode(current.PermissionMode) == domainsession.PermissionModeAsk || mode == domainsession.PermissionModeAsk {
		mode = domainsession.PermissionModeAsk
	} else {
		mode = modeForPermissions(ceiling)
	}
	if current.EnforceToolAllowlist || current.AllowedTools != nil {
		allowed = intersectTools(allowed, current.AllowedTools)
	}
	// These setters update the live store under its lock. GetSession returns a
	// snapshot, so mutating the returned object alone would not enforce limits.
	if err := factory.Memory.SetPermissionCeilingForSession(current.ID, ceiling); err != nil {
		return nil, err
	}
	if err := factory.Memory.SetPermissionModeForSession(current.ID, mode); err != nil {
		return nil, err
	}
	if err := factory.Memory.SetToolAllowlistForSession(current.ID, allowed, true); err != nil {
		return nil, err
	}
	root := strings.TrimSpace(request.WorkspaceRoot)
	if root == "" {
		root = current.WorkspaceRoot
	}
	if err := factory.Memory.SetWorkspaceForSession(current.ID, root, strings.TrimSpace(request.WorkspaceSandboxID)); err != nil {
		return nil, err
	}
	if factory.Persistence != nil {
		if err := factory.Persistence.Save(ctx, sessioncommand.SaveSession{SessionID: current.ID, Model: current.Model}); err != nil {
			return nil, err
		}
	}
	return factory.Memory.GetSession(current.ID)
}
