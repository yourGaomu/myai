package sessionapp

import (
	"context"
	"errors"
	"testing"

	domainmessage "myai/core/domain/message"
	"myai/core/llm"
	agentplan "myai/core/plan"
	repository "myai/core/port/repository"
	"myai/core/session"
)

type fakeMemoryStore struct {
	sessions       map[string]*session.Session
	currentID      string
	currentModelID string
	putCurrent     bool
}

func (s *fakeMemoryStore) GetSession(sessionID string) (*session.Session, error) {
	if current := s.sessions[sessionID]; current != nil {
		return current, nil
	}
	return nil, errors.New("session not found")
}

func (s *fakeMemoryStore) UseSession(sessionID string) error {
	s.currentID = sessionID
	return nil
}

func (s *fakeMemoryStore) PutSessionWithModeUsage(sessionID string, modelID string, agentMode session.AgentMode, permissionMode session.PermissionMode, contextWindowK int, summary string, compactedMessages int, usage llm.TokenUsage, lastUsage llm.TokenUsage, messages []domainmessage.Message) error {
	s.putCurrent = true
	s.currentID = sessionID
	s.sessions[sessionID] = &session.Session{
		ID:                sessionID,
		Model:             modelID,
		AgentMode:         agentMode,
		PermissionMode:    permissionMode,
		ContextWindowK:    contextWindowK,
		Summary:           summary,
		CompactedMessages: compactedMessages,
		Usage:             usage,
		LastUsage:         lastUsage,
		Messages:          messages,
	}
	return nil
}

func (s *fakeMemoryStore) PutSessionWithModeUsageNoCurrent(sessionID string, modelID string, agentMode session.AgentMode, permissionMode session.PermissionMode, contextWindowK int, summary string, compactedMessages int, usage llm.TokenUsage, lastUsage llm.TokenUsage, messages []domainmessage.Message) error {
	s.putCurrent = false
	s.sessions[sessionID] = &session.Session{
		ID:                sessionID,
		Model:             modelID,
		AgentMode:         agentMode,
		PermissionMode:    permissionMode,
		ContextWindowK:    contextWindowK,
		Summary:           summary,
		CompactedMessages: compactedMessages,
		Usage:             usage,
		LastUsage:         lastUsage,
		Messages:          messages,
	}
	return nil
}

func (s *fakeMemoryStore) SetCurrentPlanForSession(sessionID string, currentPlan *agentplan.Plan) error {
	current := s.sessions[sessionID]
	if current != nil {
		current.CurrentPlan = currentPlan
	}
	return nil
}

type fakeSessionGetter struct {
	record repository.SessionRecord
	err    error
}

func (r fakeSessionGetter) GetSession(context.Context, string) (repository.SessionRecord, error) {
	return r.record, r.err
}

type fakeMessageLister struct {
	records []repository.MessageRecord
	err     error
}

func (l fakeMessageLister) ListMessages(context.Context, string) ([]repository.MessageRecord, error) {
	return l.records, l.err
}

func TestLoadServiceReturnsExistingMemorySession(t *testing.T) {
	memory := &fakeMemoryStore{
		sessions: map[string]*session.Session{
			"session-1": {ID: "session-1", Model: "gpt-5"},
		},
	}

	current, err := (LoadService{Memory: memory}).EnsureInMemory(context.Background(), EnsureInMemoryCommand{
		SessionID:  "session-1",
		SetCurrent: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != "session-1" || memory.currentID != "session-1" {
		t.Fatalf("expected existing session to become current: %#v %#v", current, memory)
	}
}

func TestLoadServiceHydratesFromRepository(t *testing.T) {
	memory := &fakeMemoryStore{sessions: map[string]*session.Session{}}

	current, err := (LoadService{
		Memory: memory,
		Sessions: fakeSessionGetter{record: repository.SessionRecord{
			ID:             "session-1",
			Model:          "gpt-5",
			AgentMode:      string(session.AgentModePlan),
			PermissionMode: string(session.PermissionModeReadonly),
			ContextWindowK: 8,
			Usage:          &repository.TokenUsageRecord{TotalTokens: 12, Available: true},
		}},
		Messages: fakeMessageLister{records: []repository.MessageRecord{
			{Role: repository.RoleUser, Content: "hello"},
		}},
	}).EnsureInMemory(context.Background(), EnsureInMemoryCommand{
		SessionID:  "session-1",
		SetCurrent: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != "session-1" || current.AgentMode != session.AgentModePlan {
		t.Fatalf("unexpected hydrated session: %#v", current)
	}
	if memory.putCurrent {
		t.Fatal("expected hydration without setting current")
	}
	if len(current.Messages) != 2 || current.Messages[1].Text() != "hello" {
		t.Fatalf("unexpected messages: %#v", current.Messages)
	}
	if current.Usage.TotalTokens != 12 {
		t.Fatalf("unexpected usage: %#v", current.Usage)
	}
}
