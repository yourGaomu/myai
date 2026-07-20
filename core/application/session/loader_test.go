package sessionapp

import (
	"context"
	"errors"
	"testing"

	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
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

func (s *fakeMemoryStore) PutSessionState(state session.InitialState, setCurrent bool) error {
	s.putCurrent = setCurrent
	if setCurrent {
		s.currentID = state.ID
	}
	s.sessions[state.ID] = session.NewFromState(state)
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
			{Role: repository.RoleSystem, Content: domainmessage.RuntimeInstructionPrefix + "\nplan rules", SyntheticReason: "runtime_instruction"},
			{Role: repository.RoleUser, Content: "hello"},
			{Role: repository.RoleTool, ToolCallID: "call-1", ToolName: "shell", Content: "exit code 1", ToolStatus: "failed", ToolErrorCode: "command_exit_nonzero", ToolError: "exit code 1", ToolTruncated: true},
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
	if len(current.Messages) != 4 || !current.Messages[1].IsSyntheticReason(domainmessage.SyntheticReasonRuntimeInstruction) || current.Messages[2].Text() != "hello" {
		t.Fatalf("unexpected messages: %#v", current.Messages)
	}
	toolResult, ok := current.Messages[3].FirstToolResult()
	if !ok || toolResult.Status != domaintool.ResultStatusFailed || toolResult.ErrorCode != "command_exit_nonzero" || !toolResult.Truncated {
		t.Fatalf("unexpected restored tool result: %#v", current.Messages[3])
	}
	if current.Usage.TotalTokens != 12 {
		t.Fatalf("unexpected usage: %#v", current.Usage)
	}
}
