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
)

type memoryStore interface {
	PutSessionState(state domainsession.InitialState, setCurrent bool) error
	GetSession(sessionID string) (*domainsession.Session, error)
	RemoveSession(sessionID string) error
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
	if current, err := factory.Memory.GetSession(request.SessionID); err == nil {
		return factory.syncWorkspace(ctx, current, request)
	}
	if factory.Loader != nil {
		current, err := factory.Loader.Load(ctx, request.SessionID)
		if err == nil {
			return factory.syncWorkspace(ctx, current, request)
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
	permissionMode := capabilityPermissionMode(request.Definition.CapabilityMode)
	allowedTools := append([]string{}, request.Definition.AllowedTools...)
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
		AgentMode:            domainsession.AgentModeChat, PermissionMode: permissionMode,
		RAGSettings: domainsession.RAGSettings{Mode: domainsession.RetrievalModeOff},
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

func (factory Factory) syncWorkspace(ctx context.Context, current *domainsession.Session, request subagentport.ChildSessionRequest) (*domainsession.Session, error) {
	if current == nil {
		return nil, errors.New("subagent child session is nil")
	}
	root := strings.TrimSpace(request.WorkspaceRoot)
	sandbox := strings.TrimSpace(request.WorkspaceSandboxID)
	if (root == "" || root == current.WorkspaceRoot) && sandbox == current.WorkspaceSandboxID {
		return current, nil
	}
	if root != "" {
		current.WorkspaceRoot = root
	}
	current.WorkspaceSandboxID = sandbox
	if factory.Persistence != nil {
		if err := factory.Persistence.Save(ctx, sessioncommand.SaveSession{SessionID: current.ID, Model: current.Model}); err != nil {
			return nil, err
		}
	}
	return current, nil
}
