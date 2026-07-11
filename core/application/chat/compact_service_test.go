package chat

import (
	"context"
	"errors"
	"testing"

	"myai/core/contextmgr"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
	"myai/core/session"
)

func TestCompactServiceCompactSessionSummarizesAndSaves(t *testing.T) {
	current := compactTestSession()
	model := compactModel{}
	summarizer := &compactSummarizer{summary: "new summary"}
	store := &compactStore{}

	err := CompactService{
		Summarizer: summarizer,
		Summaries:  store,
		KeepChunks: 2,
	}.CompactSession(context.Background(), current, model)
	if err != nil {
		t.Fatalf("CompactSession returned error: %v", err)
	}

	if summarizer.model != model {
		t.Fatalf("expected model to be forwarded to summarizer")
	}
	if summarizer.existingSummary != "old summary" {
		t.Fatalf("expected existing summary to be forwarded, got %q", summarizer.existingSummary)
	}
	if len(summarizer.messages) != 4 {
		t.Fatalf("expected four compacted messages, got %d", len(summarizer.messages))
	}
	if store.summary != "new summary" || store.compactedMessages != 5 {
		t.Fatalf("expected saved summary and cutoff, got summary=%q cutoff=%d", store.summary, store.compactedMessages)
	}
	if current.Summary != "new summary" || current.CompactedMessages != 5 {
		t.Fatalf("expected store to update session state, got summary=%q compacted=%d", current.Summary, current.CompactedMessages)
	}
}

func TestCompactServiceCompactIfNeededSkipsBelowThreshold(t *testing.T) {
	summarizer := &compactSummarizer{summary: "new summary"}

	info, err := CompactService{
		Contexts:   &compactContextProvider{infos: []contextmgr.Info{{WindowK: 16, SelectedTokens: 1}}},
		Summarizer: summarizer,
		Summaries:  &compactStore{},
		KeepChunks: 2,
	}.CompactIfNeeded(context.Background(), compactTestSession(), compactModel{}, "runtime")
	if err != nil {
		t.Fatalf("CompactIfNeeded returned error: %v", err)
	}
	if info.Triggered {
		t.Fatalf("expected no compact info, got %#v", info)
	}
	if summarizer.called {
		t.Fatal("expected summarizer not to be called")
	}
}

func TestCompactServiceCompactIfNeededReturnsCompactInfo(t *testing.T) {
	contexts := &compactContextProvider{infos: []contextmgr.Info{
		{WindowK: 16, SelectedTokens: 12000, Truncated: true},
		{
			WindowK:           16,
			SelectedTokens:    100,
			CompactedMessages: 5,
			SummaryTokens:     10,
			SummaryVersion:    5,
			SummaryHash:       "summary-hash",
			PrefixHash:        "prefix-hash",
			CacheableTokens:   88,
		},
	}}

	info, err := CompactService{
		Contexts:   contexts,
		Summarizer: &compactSummarizer{summary: "new summary"},
		Summaries:  &compactStore{},
		KeepChunks: 2,
	}.CompactIfNeeded(context.Background(), compactTestSession(), compactModel{}, "runtime")
	if err != nil {
		t.Fatalf("CompactIfNeeded returned error: %v", err)
	}

	if !info.Triggered || info.Reason != "window_limit" {
		t.Fatalf("expected triggered window_limit compact info, got %#v", info)
	}
	if info.BeforeTokens != 12000 || info.AfterTokens != 100 || info.NewMessages != 5 {
		t.Fatalf("expected before/after token and message counts, got %#v", info)
	}
	if info.SummaryHash != "summary-hash" || info.PrefixHash != "prefix-hash" || info.CacheableTokens != 88 {
		t.Fatalf("expected snapshot metadata to be copied, got %#v", info)
	}
	if contexts.calls != 2 {
		t.Fatalf("expected before and after snapshots, got %d", contexts.calls)
	}
}

func TestCompactServiceCompactIfNeededIgnoresNotEnoughHistory(t *testing.T) {
	current := &session.Session{
		ID: "session-1",
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleSystem, "system"),
			domainmessage.Text(domainmessage.RoleUser, "hello"),
		},
	}

	info, err := CompactService{
		Contexts:   &compactContextProvider{infos: []contextmgr.Info{{WindowK: 16, SelectedTokens: 12000, Truncated: true}}},
		Summarizer: &compactSummarizer{summary: "new summary"},
		Summaries:  &compactStore{},
		KeepChunks: 2,
	}.CompactIfNeeded(context.Background(), current, compactModel{}, "runtime")
	if err != nil {
		t.Fatalf("CompactIfNeeded returned error: %v", err)
	}
	if info.Triggered {
		t.Fatalf("expected empty compact info, got %#v", info)
	}
}

func TestCompactServiceCompactSessionReturnsNotEnoughHistory(t *testing.T) {
	current := &session.Session{
		ID: "session-1",
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleSystem, "system"),
			domainmessage.Text(domainmessage.RoleUser, "hello"),
		},
	}

	err := CompactService{
		Summarizer: &compactSummarizer{summary: "new summary"},
		Summaries:  &compactStore{},
		KeepChunks: 2,
	}.CompactSession(context.Background(), current, compactModel{})
	if !errors.Is(err, ErrNotEnoughHistoryToCompact) {
		t.Fatalf("expected ErrNotEnoughHistoryToCompact, got %v", err)
	}
}

type compactModel struct{}

func (compactModel) Generate(ctx context.Context, request modelport.GenerateRequest) (modelport.ChatResult, error) {
	return modelport.ChatResult{}, errors.New("compactModel should not be called directly")
}

type compactSummarizer struct {
	called          bool
	model           modelport.ChatModelPort
	existingSummary string
	messages        []domainmessage.Message
	summary         string
	err             error
}

func (s *compactSummarizer) Summarize(ctx context.Context, model modelport.ChatModelPort, existingSummary string, messages []domainmessage.Message) (string, error) {
	s.called = true
	s.model = model
	s.existingSummary = existingSummary
	s.messages = messages
	if s.err != nil {
		return "", s.err
	}
	return s.summary, nil
}

type compactStore struct {
	summary           string
	compactedMessages int
	err               error
}

func (s *compactStore) SaveSummary(ctx context.Context, current *session.Session, summary string, compactedMessages int) error {
	if s.err != nil {
		return s.err
	}
	s.summary = summary
	s.compactedMessages = compactedMessages
	current.Summary = summary
	current.CompactedMessages = compactedMessages
	return nil
}

type compactContextProvider struct {
	infos []contextmgr.Info
	calls int
}

func (p *compactContextProvider) Snapshot(current *session.Session, runtimePrompt string) contextmgr.Snapshot {
	index := p.calls
	p.calls++
	if index >= len(p.infos) {
		index = len(p.infos) - 1
	}
	return contextmgr.Snapshot{Info: p.infos[index], Messages: domainmessage.CloneMessages(current.Messages)}
}

func compactTestSession() *session.Session {
	return &session.Session{
		ID:                "session-1",
		Model:             "model-a",
		Summary:           "old summary",
		CompactedMessages: 1,
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleSystem, "system"),
			domainmessage.Text(domainmessage.RoleUser, "u1"),
			domainmessage.Text(domainmessage.RoleAssistant, "a1"),
			domainmessage.Text(domainmessage.RoleUser, "u2"),
			domainmessage.Text(domainmessage.RoleAssistant, "a2"),
			domainmessage.Text(domainmessage.RoleUser, "u3"),
			domainmessage.Text(domainmessage.RoleAssistant, "a3"),
		},
	}
}
