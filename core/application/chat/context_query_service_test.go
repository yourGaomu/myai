package chat

import (
	"context"
	"testing"

	"myai/core/contextmgr"
	domainmessage "myai/core/domain/message"
	"myai/core/session"
)

func TestContextQueryServiceBuildsInfoFromPersistedMessages(t *testing.T) {
	contexts := &queryContextProvider{info: contextmgr.Info{WindowK: 32, SelectedTokens: 9}}

	info := (ContextQueryService{
		Contexts: contexts,
	}).Info(context.Background(), &session.Session{ID: "session-1"})

	if info.WindowK != 32 || info.SelectedTokens != 9 {
		t.Fatalf("unexpected context info: %#v", info)
	}
	if contexts.calls != 1 {
		t.Fatalf("expected one snapshot call, got %d", contexts.calls)
	}
}

func TestContextQueryServiceReturnsDefaultForNilSession(t *testing.T) {
	info := (ContextQueryService{}).Info(context.Background(), nil)

	if info.WindowK != contextmgr.DefaultWindowK {
		t.Fatalf("expected default context info, got %#v", info)
	}
}

func TestContextQueryServiceReturnsDefaultWithoutContextProvider(t *testing.T) {
	info := (ContextQueryService{}).Info(context.Background(), &session.Session{ID: "session-1"})

	if info.WindowK != contextmgr.DefaultWindowK {
		t.Fatalf("expected default context info, got %#v", info)
	}
}

type queryContextProvider struct {
	info  contextmgr.Info
	calls int
}

func (p *queryContextProvider) Snapshot(current *session.Session) contextmgr.Snapshot {
	p.calls++
	return contextmgr.Snapshot{
		Info:     p.info,
		Messages: []domainmessage.Message{},
	}
}
