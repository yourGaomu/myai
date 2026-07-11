package chat

import (
	"context"
	"testing"

	"myai/core/contextmgr"
	domainmessage "myai/core/domain/message"
	"myai/core/session"
)

func TestContextQueryServiceBuildsInfoWithRuntimePrompt(t *testing.T) {
	contexts := &queryContextProvider{info: contextmgr.Info{WindowK: 32, SelectedTokens: 9}}
	runtime := &queryRuntimeProvider{prompt: "runtime prompt"}

	info := (ContextQueryService{
		Contexts:            contexts,
		RuntimeInstructions: runtime,
	}).Info(context.Background(), &session.Session{ID: "session-1"})

	if info.WindowK != 32 || info.SelectedTokens != 9 {
		t.Fatalf("unexpected context info: %#v", info)
	}
	if contexts.runtimePrompt != "runtime prompt" {
		t.Fatalf("expected runtime prompt to be forwarded, got %q", contexts.runtimePrompt)
	}
	if runtime.input != "" || runtime.forceChatMode {
		t.Fatalf("expected neutral runtime prompt request, got %#v", runtime)
	}
}

func TestContextQueryServiceReturnsDefaultForNilSession(t *testing.T) {
	info := (ContextQueryService{}).Info(context.Background(), nil)

	if info.WindowK != contextmgr.DefaultWindowK {
		t.Fatalf("expected default context info, got %#v", info)
	}
}

func TestContextQueryServiceReturnsDefaultWithoutContextProvider(t *testing.T) {
	info := (ContextQueryService{}).InfoWithRuntimePrompt(&session.Session{ID: "session-1"}, "runtime")

	if info.WindowK != contextmgr.DefaultWindowK {
		t.Fatalf("expected default context info, got %#v", info)
	}
}

type queryContextProvider struct {
	info          contextmgr.Info
	runtimePrompt string
}

func (p *queryContextProvider) Snapshot(current *session.Session, runtimePrompt string) contextmgr.Snapshot {
	p.runtimePrompt = runtimePrompt
	return contextmgr.Snapshot{
		Info:     p.info,
		Messages: []domainmessage.Message{},
	}
}

type queryRuntimeProvider struct {
	prompt        string
	input         string
	forceChatMode bool
}

func (p *queryRuntimeProvider) Prompt(ctx context.Context, current *session.Session, input string, forceChatMode bool) string {
	p.input = input
	p.forceChatMode = forceChatMode
	return p.prompt
}
