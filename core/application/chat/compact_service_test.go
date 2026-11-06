package chat

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"myai/core/contextmgr"
	compaction "myai/core/domain/compaction"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
	"myai/core/session"
)

func TestCompactServiceCompactSessionSummarizesAndSaves(t *testing.T) {
	current := compactTestSession()
	original := domainmessage.CloneAll(current.Messages)
	model := compactModel{}
	summary := compactSummaryJSON(t)
	summarizer := &compactSummarizer{summary: summary}
	store := &compactStore{}

	err := (CompactService{Summarizer: summarizer, Summaries: store, KeepChunks: 2}).CompactSession(context.Background(), current, model)
	if err != nil {
		t.Fatalf("CompactSession returned error: %v", err)
	}
	if summarizer.model != model || summarizer.existingSummary != "old summary" {
		t.Fatalf("model or previous checkpoint was not forwarded: %#v", summarizer)
	}
	if !reflect.DeepEqual(summarizer.messages, original[1:3]) {
		t.Fatalf("expected the first complete turn to be summarized, got %#v", summarizer.messages)
	}
	if store.summary != summary || store.compactedMessages != 3 || current.Summary != summary || current.CompactedMessages != 3 {
		t.Fatalf("unexpected saved summary/cursor: store=%#v current=%#v", store, current)
	}
	if !reflect.DeepEqual(current.Messages, original) {
		t.Fatal("compaction changed the original transcript")
	}
	wantHash := contextmgr.CompactionSourceHash(original, 3)
	if store.sourceHash != wantHash || current.CompactionCheckpoint == nil || !contextmgr.CompactionCheckpointMatchesCheckpoint(current.Messages, current.CompactionCheckpoint) {
		t.Fatal("saved checkpoint does not identify the original compacted prefix")
	}
}

func TestCompactServiceCompactIfNeededSkipsBelowThreshold(t *testing.T) {
	current := compactTestSession()
	options := contextmgr.RequestOptions{Budget: contextmgr.NewRequestBudget(16000, 1200, 2048)}
	before := compactRequestSnapshot(current, options)
	if before.Info.Truncated || before.Info.SelectedTokens+options.Budget.ReservedTokens() >= 11200 {
		t.Fatal("fixture must fit below the request compaction threshold")
	}
	summarizer, store := &compactSummarizer{summary: compactSummaryJSON(t)}, &compactStore{}
	info, err := (CompactService{Summarizer: summarizer, Summaries: store, KeepChunks: 2}).CompactIfNeededForRequest(context.Background(), current, compactModel{}, options)
	if err != nil || info.Triggered || summarizer.called || store.saved {
		t.Fatalf("small request unexpectedly compacted: info=%#v err=%v", info, err)
	}
}

func TestCompactServiceCountsReservedRequestTokens(t *testing.T) {
	original := compactSizedTestSession(1500)
	budget := contextmgr.NewRequestBudget(16000, 800, 2200)
	options := contextmgr.RequestOptions{Budget: budget}
	before := compactRequestSnapshot(original, options)
	if before.Info.Truncated || before.Info.SelectedTokens >= 11200 || before.Info.SelectedTokens+budget.ReservedTokens() < 11200 {
		t.Fatalf("fixture must cross the threshold only with tool/output reservation: %#v budget=%#v", before.Info, budget)
	}
	skip := &compactSummarizer{summary: compactSummaryJSON(t)}
	info, err := (CompactService{Summarizer: skip, Summaries: &compactStore{}, KeepChunks: 2}).CompactIfNeeded(context.Background(), session.Clone(original), compactModel{})
	if err != nil || info.Triggered || skip.called {
		t.Fatalf("same history should not compact without reservations: info=%#v err=%v", info, err)
	}
	for _, entry := range []string{"request options", "legacy reserved tokens"} {
		t.Run(entry, func(t *testing.T) {
			current := session.Clone(original)
			summarizer, store := &compactSummarizer{summary: compactSummaryJSON(t)}, &compactStore{}
			service := CompactService{Summarizer: summarizer, Summaries: store, KeepChunks: 2}
			var info CompactInfo
			var err error
			if entry == "request options" {
				info, err = service.CompactIfNeededForRequest(context.Background(), current, compactModel{}, options)
			} else {
				info, err = service.CompactIfNeededWithBudget(context.Background(), current, compactModel{}, budget.ReservedTokens())
			}
			if err != nil || !info.Triggered || info.Failed || info.Reason != "threshold" || !summarizer.called || !store.saved {
				t.Fatalf("tools/output reservation did not trigger successful compaction: info=%#v err=%v", info, err)
			}
			if err := budget.ValidateSnapshot(compactRequestSnapshot(current, options)); err != nil {
				t.Fatalf("compaction left a request outside the shared budget: %v", err)
			}
		})
	}
}

func TestCompactServiceCompactIfNeededReturnsCompactInfo(t *testing.T) {
	current := compactSizedTestSession(3000)
	original := domainmessage.CloneAll(current.Messages)
	options := contextmgr.RequestOptions{
		Budget:      contextmgr.NewRequestBudget(16000, 1200, 2048),
		TurnContext: contextmgr.TurnContext{RuntimeInstruction: "current chat rules"},
	}
	before := compactRequestSnapshot(current, options)
	if !before.Info.Truncated || before.Info.DroppedMessages == 0 {
		t.Fatal("fixture must need a checkpoint to avoid dropping original history")
	}
	store := &compactStore{}
	info, err := (CompactService{Summarizer: &compactSummarizer{summary: compactSummaryJSON(t)}, Summaries: store, KeepChunks: 2}).CompactIfNeededForRequest(context.Background(), current, compactModel{}, options)
	if err != nil {
		t.Fatalf("CompactIfNeededForRequest returned error: %v", err)
	}
	after := compactRequestSnapshot(current, options)
	if !info.Triggered || info.Failed || info.Truncated || info.Reason != "window_limit" || info.DroppedMessages != 0 {
		t.Fatalf("expected successful visible window compaction without a history gap, got %#v", info)
	}
	if err := options.Budget.ValidateSnapshot(after); err != nil {
		t.Fatalf("successful compaction left an invalid request: %v", err)
	}
	if info.BeforeTokens != before.Info.SelectedTokens || info.AfterTokens != after.Info.SelectedTokens || info.AfterTokens >= info.BeforeTokens {
		t.Fatalf("compact info does not measure the actual request before and after compaction: %#v", info)
	}
	if info.NewMessages <= 0 || info.NewMessages != after.Info.CompactedMessages-before.Info.CompactedMessages || info.CompactedMessages != after.Info.CompactedMessages || info.SummaryVersion != after.Info.SummaryVersion {
		t.Fatalf("compact info does not report the summarized history range: %#v", info)
	}
	if info.SummaryHash == "" || info.SummaryHash != after.Info.SummaryHash || info.PrefixHash != after.Info.PrefixHash || info.CacheableTokens != after.Info.CacheableTokens || info.SummaryTokens != after.Info.SummaryTokens {
		t.Fatalf("compact info does not describe the actual summary/cache prefix: %#v", info)
	}
	if info.Checkpoint == nil || info.Checkpoint.SourceEndMessage != store.compactedMessages || info.Checkpoint.SourceHistoryHash != contextmgr.CompactionSourceHash(original, store.compactedMessages) || !reflect.DeepEqual(current.Messages, original) {
		t.Fatal("reported checkpoint changed the original transcript or source hash semantics")
	}
}

func TestCompactServiceCompactIfNeededSkipsInsufficientHistoryOnlyWhenRequestFits(t *testing.T) {
	for _, text := range []string{"hello", strings.Repeat("文", 8500)} {
		t.Run(stringLengthName(text), func(t *testing.T) {
			current := &session.Session{ID: "session-1", Messages: []domainmessage.Message{
				domainmessage.Text(domainmessage.RoleSystem, "system"), domainmessage.Text(domainmessage.RoleUser, text),
			}}
			options := contextmgr.RequestOptions{Budget: contextmgr.NewRequestBudget(16000, 800, 2200)}
			before := compactRequestSnapshot(current, options)
			if err := options.Budget.ValidateSnapshot(before); err != nil {
				t.Fatalf("fixture must fit without losing history: %v", err)
			}
			if text != "hello" && before.Info.SelectedTokens+options.Budget.ReservedTokens() < 11200 {
				t.Fatal("large single input must cross the threshold while still fitting")
			}
			summarizer, store := &compactSummarizer{summary: compactSummaryJSON(t)}, &compactStore{}
			info, err := (CompactService{Summarizer: summarizer, Summaries: store, KeepChunks: 2}).CompactIfNeededForRequest(context.Background(), current, compactModel{}, options)
			if err != nil || info.Triggered || summarizer.called || store.saved {
				t.Fatalf("a fitting input without compactable history must remain intact: info=%#v err=%v", info, err)
			}
		})
	}
}

func TestCompactServiceCompactIfNeededReportsUncompactableOversizedInput(t *testing.T) {
	current := &session.Session{ID: "session-1", Messages: []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "system"), domainmessage.Text(domainmessage.RoleUser, strings.Repeat("文", 14000)),
	}}
	original := domainmessage.CloneAll(current.Messages)
	summarizer, store := &compactSummarizer{summary: compactSummaryJSON(t)}, &compactStore{}
	info, err := (CompactService{Summarizer: summarizer, Summaries: store}).CompactIfNeededForRequest(context.Background(), current, compactModel{}, contextmgr.RequestOptions{Budget: contextmgr.NewRequestBudget(16000, 800, 2200)})
	if !errors.Is(err, ErrNotEnoughHistoryToCompact) || !info.Triggered || !info.Failed || !info.Truncated || info.Error == "" {
		t.Fatalf("oversized input must report a visible compaction failure: info=%#v err=%v", info, err)
	}
	if summarizer.called || store.saved || current.CompactedMessages != 0 || !reflect.DeepEqual(current.Messages, original) {
		t.Fatal("uncompactable request changed the transcript or checkpoint")
	}
}

func TestCompactServiceCompactSessionReturnsNotEnoughHistory(t *testing.T) {
	current := &session.Session{ID: "session-1", Messages: []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "system"), domainmessage.Text(domainmessage.RoleUser, "hello"),
	}}
	err := (CompactService{Summarizer: &compactSummarizer{summary: compactSummaryJSON(t)}, Summaries: &compactStore{}, KeepChunks: 2}).CompactSession(context.Background(), current, compactModel{})
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
	s.messages = domainmessage.CloneAll(messages)
	if s.err != nil {
		return "", s.err
	}
	return s.summary, nil
}

type compactStore struct {
	summary           string
	compactedMessages int
	sourceHash        string
	saved             bool
	err               error
}

func (s *compactStore) SaveSummary(ctx context.Context, current *session.Session, summary string, compactedMessages int) error {
	if s.err != nil {
		return s.err
	}
	s.saved = true
	s.summary, s.compactedMessages = summary, compactedMessages
	current.Summary, current.CompactedMessages = summary, compactedMessages
	return nil
}

func (s *compactStore) SaveSummaryWithCheckpoint(ctx context.Context, current *session.Session, summary string, cutoff int, sourceHash string) error {
	checkpoint, err := compaction.NewCheckpoint(summary, 0, cutoff, sourceHash, time.Time{})
	if err != nil {
		return err
	}
	if err := s.SaveSummary(ctx, current, summary, cutoff); err != nil {
		return err
	}
	s.sourceHash, current.CompactionSourceHash = sourceHash, sourceHash
	current.CompactionCheckpoint = &checkpoint
	return nil
}

func compactSummaryJSON(t *testing.T) string {
	t.Helper()
	summary, err := compaction.EncodeJSON(compaction.Summary{CurrentGoal: "continue the current task", CompletedWork: []string{"earlier conversation summarized"}})
	if err != nil {
		t.Fatal(err)
	}
	return summary
}

func compactRequestSnapshot(current *session.Session, options contextmgr.RequestOptions) contextmgr.Snapshot {
	checkpoint := compaction.LegacyCheckpoint(current.Summary, current.CompactedMessages, current.CompactionSourceHash)
	if current.CompactionCheckpoint != nil {
		checkpoint = *current.CompactionCheckpoint.Clone()
	}
	return contextmgr.BuildSnapshotForRequest(current.Messages, checkpoint, checkpoint.SourceEndMessage, options)
}

func compactSizedTestSession(tokensPerMessage int) *session.Session {
	current := compactTestSession()
	for index := 1; index < len(current.Messages); index++ {
		current.Messages[index] = domainmessage.Text(current.Messages[index].Role, strings.Repeat("文", tokensPerMessage))
	}
	return current
}

func stringLengthName(text string) string {
	if text == "hello" {
		return "below threshold"
	}
	return "above threshold but within request budget"
}

func compactTestSession() *session.Session {
	return &session.Session{
		ID: "session-1", Model: "model-a", Summary: "old summary", CompactedMessages: 1,
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleSystem, "system"),
			domainmessage.Text(domainmessage.RoleUser, "u1"), domainmessage.Text(domainmessage.RoleAssistant, "a1"),
			domainmessage.Text(domainmessage.RoleUser, "u2"), domainmessage.Text(domainmessage.RoleAssistant, "a2"),
			domainmessage.Text(domainmessage.RoleUser, "u3"), domainmessage.Text(domainmessage.RoleAssistant, "a3"),
		},
	}
}
