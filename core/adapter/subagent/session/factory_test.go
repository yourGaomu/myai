package session

import (
	"context"
	"errors"
	"testing"

	domainsubagent "myai/core/domain/subagent"
	repository "myai/core/port/repository"
	subagentport "myai/core/port/subagent"
	domainsession "myai/core/session"
)

type factoryMemory struct {
	sessions map[string]*domainsession.Session
	puts     int
}

func (memory *factoryMemory) GetSession(sessionID string) (*domainsession.Session, error) {
	if current := memory.sessions[sessionID]; current != nil {
		return current, nil
	}
	return nil, errors.New("session not found")
}

func (memory *factoryMemory) PutSessionState(state domainsession.InitialState, _ bool) error {
	memory.puts++
	memory.sessions[state.ID] = domainsession.NewFromState(state)
	return nil
}

func (memory *factoryMemory) RemoveSession(sessionID string) error {
	delete(memory.sessions, sessionID)
	return nil
}

type factoryLoader struct {
	session *domainsession.Session
	err     error
	calls   int
}

func (loader *factoryLoader) Load(context.Context, string) (*domainsession.Session, error) {
	loader.calls++
	return loader.session, loader.err
}

func TestFactoryReusesExistingChildSession(t *testing.T) {
	existing := &domainsession.Session{ID: "child-1", Model: "model-from-history"}
	memory := &factoryMemory{sessions: map[string]*domainsession.Session{"child-1": existing}}
	loader := &factoryLoader{err: errors.New("must not load")}

	current, err := (Factory{Memory: memory, Loader: loader}).Create(context.Background(), childSessionRequest("child-1"))
	if err != nil {
		t.Fatal(err)
	}
	if current != existing || loader.calls != 0 || memory.puts != 0 {
		t.Fatalf("existing child session was replaced: current=%#v loader_calls=%d puts=%d", current, loader.calls, memory.puts)
	}
}

func TestFactoryLoadsPersistedChildSessionWithoutReplacingIt(t *testing.T) {
	existing := &domainsession.Session{ID: "child-1", Model: "model-from-history"}
	memory := &factoryMemory{sessions: map[string]*domainsession.Session{}}
	loader := &factoryLoader{session: existing}

	current, err := (Factory{Memory: memory, Loader: loader}).Create(context.Background(), childSessionRequest("child-1"))
	if err != nil {
		t.Fatal(err)
	}
	if current != existing || loader.calls != 1 || memory.puts != 0 {
		t.Fatalf("persisted child session was replaced: current=%#v loader_calls=%d puts=%d", current, loader.calls, memory.puts)
	}
}

func TestFactorySyncsWorkspaceIdentityOnReuse(t *testing.T) {
	existing := &domainsession.Session{ID: "child-1", Model: "model-from-history", WorkspaceRoot: "old-root", WorkspaceSandboxID: "old-sandbox"}
	memory := &factoryMemory{sessions: map[string]*domainsession.Session{"child-1": existing}}

	request := childSessionRequest("child-1")
	request.WorkspaceRoot = "new-root"
	request.WorkspaceSandboxID = "new-sandbox"
	current, err := (Factory{Memory: memory}).Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if current.WorkspaceRoot != "new-root" || current.WorkspaceSandboxID != "new-sandbox" || memory.puts != 0 {
		t.Fatalf("child session workspace was not synced: %#v puts=%d", current, memory.puts)
	}
}

func TestFactoryCreatesChildSessionWhenPersistenceDoesNotContainIt(t *testing.T) {
	memory := &factoryMemory{sessions: map[string]*domainsession.Session{}}
	loader := &factoryLoader{err: repository.ErrNotFound}

	current, err := (Factory{Memory: memory, Loader: loader}).Create(context.Background(), childSessionRequest("child-new"))
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != "child-new" || current.Model != "model-1" || loader.calls != 1 || memory.puts != 1 {
		t.Fatalf("new child session was not created: current=%#v loader_calls=%d puts=%d", current, loader.calls, memory.puts)
	}
}

func childSessionRequest(sessionID string) subagentport.ChildSessionRequest {
	return subagentport.ChildSessionRequest{
		SessionID: sessionID,
		Definition: domainsubagent.DefinitionSnapshot{
			ID: "coder", Name: "Coder", ModelID: "model-1",
			CapabilityMode: domainsubagent.CapabilityModeReadOnly,
		},
	}
}
