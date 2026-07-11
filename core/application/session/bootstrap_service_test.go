package sessionapp

import (
	"context"
	"errors"
	"testing"

	repository "myai/core/port/repository"
	"myai/core/session"
)

func TestBootstrapSessionServiceLoadsCachedSession(t *testing.T) {
	cache := &bootstrapSessionCache{sessionID: " cached-session "}
	lifecycle := &bootstrapSessionLifecycle{loaded: &session.Session{ID: "cached-session", Model: "gpt-5"}}
	persistence := &bootstrapSessionPersistence{}

	result, err := (BootstrapSessionService{
		Cache:       cache,
		Lifecycle:   lifecycle,
		State:       bootstrapSessionState{},
		Persistence: persistence,
	}).Bootstrap(context.Background(), BootstrapSessionCommand{NewSessionTitle: "New chat"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != BootstrapSessionLoaded || result.Session.ID != "cached-session" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if lifecycle.loadedID != "cached-session" || cache.savedID != "cached-session" {
		t.Fatalf("expected cached session load and current cache refresh: lifecycle=%#v cache=%#v", lifecycle, cache)
	}
	if persistence.called {
		t.Fatal("cached load should not rewrite session persistence")
	}
}

func TestBootstrapSessionServiceCreatesWhenCacheIsStale(t *testing.T) {
	cache := &bootstrapSessionCache{sessionID: "missing"}
	lifecycle := &bootstrapSessionLifecycle{
		loadErr: repository.ErrNotFound,
		created: &session.Session{ID: "new-session", Model: "gpt-5"},
	}
	persistence := &bootstrapSessionPersistence{}

	result, err := (BootstrapSessionService{
		Cache:       cache,
		Lifecycle:   lifecycle,
		State:       bootstrapSessionState{err: errors.New("session not found")},
		Persistence: persistence,
	}).Bootstrap(context.Background(), BootstrapSessionCommand{NewSessionTitle: "New chat"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != BootstrapSessionCreated || result.Session.ID != "new-session" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !lifecycle.newCalled || cache.savedID != "new-session" {
		t.Fatalf("expected new current session: lifecycle=%#v cache=%#v", lifecycle, cache)
	}
	if !persistence.called || persistence.command.Title != "New chat" {
		t.Fatalf("expected new session persistence: %#v", persistence)
	}
}

func TestBootstrapSessionServiceReusesCurrentMemorySession(t *testing.T) {
	current := &session.Session{ID: "current-session", Model: "gpt-5"}
	cache := &bootstrapSessionCache{}
	persistence := &bootstrapSessionPersistence{}

	result, err := (BootstrapSessionService{
		Cache:       cache,
		Lifecycle:   &bootstrapSessionLifecycle{},
		State:       bootstrapSessionState{current: current},
		Persistence: persistence,
	}).Bootstrap(context.Background(), BootstrapSessionCommand{NewSessionTitle: "New chat"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != BootstrapSessionReused || result.Session != current {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !persistence.called || persistence.command.SessionID != "current-session" {
		t.Fatalf("expected current session persistence: %#v", persistence)
	}
	if cache.savedID != "" {
		t.Fatalf("reused session should preserve existing cache behavior, got %q", cache.savedID)
	}
}

type bootstrapSessionCache struct {
	sessionID string
	savedID   string
	err       error
}

func (c *bootstrapSessionCache) Get(context.Context) (string, error) {
	return c.sessionID, c.err
}

func (c *bootstrapSessionCache) Save(_ context.Context, sessionID string) error {
	c.savedID = sessionID
	return c.err
}

type bootstrapSessionLifecycle struct {
	loaded    *session.Session
	created   *session.Session
	loadedID  string
	loadErr   error
	newErr    error
	newCalled bool
}

func (l *bootstrapSessionLifecycle) LoadSession(_ context.Context, sessionID string) (*session.Session, error) {
	l.loadedID = sessionID
	return l.loaded, l.loadErr
}

func (l *bootstrapSessionLifecycle) NewSession(context.Context) (*session.Session, error) {
	l.newCalled = true
	return l.created, l.newErr
}

type bootstrapSessionState struct {
	current *session.Session
	err     error
}

func (s bootstrapSessionState) CurrentSession() (*session.Session, error) {
	return s.current, s.err
}

type bootstrapSessionPersistence struct {
	command SaveSessionCommand
	called  bool
	err     error
}

func (p *bootstrapSessionPersistence) Save(_ context.Context, command SaveSessionCommand) error {
	p.called = true
	p.command = command
	return p.err
}
