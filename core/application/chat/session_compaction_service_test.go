package chat

import (
	"context"
	"errors"
	"testing"

	"myai/core/contextmgr"
	modelport "myai/core/port/model"
	"myai/core/session"
)

func TestSessionCompactionServiceCoordinatesCompaction(t *testing.T) {
	current := &session.Session{ID: "session-1", Model: "model-a"}
	model := compactModel{}
	loader := &compactionSessionLoader{current: current}
	models := compactionModelProvider{model: model}
	compactor := &recordingSessionCompactor{}
	contexts := compactionContextQuery{info: contextmgr.Info{WindowK: 16, SelectedTokens: 8}}

	info, err := (SessionCompactionService{
		Sessions:  loader,
		Models:    models,
		Compactor: compactor,
		Contexts:  contexts,
	}).Compact(context.Background(), CompactSessionCommand{SessionID: " session-1 "})
	if err != nil {
		t.Fatal(err)
	}
	if loader.sessionID != "session-1" || compactor.current != current || compactor.model != model {
		t.Fatalf("unexpected compaction coordination: loader=%#v compactor=%#v", loader, compactor)
	}
	if info.SelectedTokens != 8 {
		t.Fatalf("unexpected context info: %#v", info)
	}
}

func TestSessionCompactionServiceReturnsContextForInsufficientHistory(t *testing.T) {
	current := &session.Session{ID: "session-1", Model: "model-a"}
	expectedInfo := contextmgr.Info{WindowK: 16, SelectedTokens: 4}

	info, err := (SessionCompactionService{
		Sessions:  &compactionSessionLoader{current: current},
		Models:    compactionModelProvider{model: compactModel{}},
		Compactor: &recordingSessionCompactor{err: ErrNotEnoughHistoryToCompact},
		Contexts:  compactionContextQuery{info: expectedInfo},
	}).Compact(context.Background(), CompactSessionCommand{SessionID: "session-1"})
	if !errors.Is(err, ErrNotEnoughHistoryToCompact) {
		t.Fatalf("expected insufficient history error, got %v", err)
	}
	if info != expectedInfo {
		t.Fatalf("expected current context info, got %#v", info)
	}
}

func TestSessionCompactionServiceRejectsUnknownModel(t *testing.T) {
	_, err := (SessionCompactionService{
		Sessions:  &compactionSessionLoader{current: &session.Session{ID: "session-1", Model: "missing"}},
		Models:    compactionModelProvider{},
		Compactor: &recordingSessionCompactor{},
		Contexts:  compactionContextQuery{},
	}).Compact(context.Background(), CompactSessionCommand{SessionID: "session-1"})
	if err == nil || err.Error() != "model not found: missing" {
		t.Fatalf("expected model not found error, got %v", err)
	}
}

type compactionSessionLoader struct {
	current   *session.Session
	sessionID string
	err       error
}

func (l *compactionSessionLoader) Load(_ context.Context, sessionID string) (*session.Session, error) {
	l.sessionID = sessionID
	return l.current, l.err
}

type compactionModelProvider struct {
	model modelport.ChatModelPort
}

func (p compactionModelProvider) GetModel(string) modelport.ChatModelPort {
	return p.model
}

type recordingSessionCompactor struct {
	current *session.Session
	model   modelport.ChatModelPort
	err     error
}

func (c *recordingSessionCompactor) CompactSession(_ context.Context, current *session.Session, model modelport.ChatModelPort) error {
	c.current = current
	c.model = model
	return c.err
}

type compactionContextQuery struct {
	info contextmgr.Info
}

func (q compactionContextQuery) Info(context.Context, *session.Session) contextmgr.Info {
	return q.info
}
