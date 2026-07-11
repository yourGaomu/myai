package sessionapp

import (
	"context"
	"errors"
	"fmt"
	"testing"

	domainmessage "myai/core/domain/message"
	repository "myai/core/port/repository"
	"myai/core/session"
)

func TestMessageQueryServiceUsesMemoryWhenStoreMissing(t *testing.T) {
	service := MessageQueryService{
		MemoryRecords: fakeMemoryMessageRecordMapper{},
		Memory: fakeMemorySessions{
			sessions: map[string]*session.Session{
				"session-1": memoryQuerySession("session-1"),
			},
		},
	}

	records, err := service.ListMessages(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Role != repository.RoleUser || records[0].Content != "hello" {
		t.Fatalf("expected memory messages, got %#v", records)
	}

	meta, err := service.HistoryMeta(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if meta.SessionID != "session-1" || meta.MessageCount != 1 {
		t.Fatalf("expected memory history meta, got %#v", meta)
	}
}

func TestMessageQueryServicePrefersStoreMessages(t *testing.T) {
	store := &fakeMessageQueryStore{
		messages: []repository.MessageRecord{{ID: "stored", Role: repository.RoleAssistant}},
	}
	service := MessageQueryService{
		Store:         store,
		MemoryRecords: fakeMemoryMessageRecordMapper{},
		Memory: fakeMemorySessions{
			sessions: map[string]*session.Session{
				"session-1": memoryQuerySession("session-1"),
			},
		},
	}

	records, err := service.ListMessages(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}

	if !store.checkedSession || len(records) != 1 || records[0].ID != "stored" {
		t.Fatalf("expected store messages after session check, checked=%v records=%#v", store.checkedSession, records)
	}
}

func TestMessageQueryServiceFallsBackToMemoryWhenStoreMessagesAreEmpty(t *testing.T) {
	service := MessageQueryService{
		Store:         &fakeMessageQueryStore{},
		MemoryRecords: fakeMemoryMessageRecordMapper{},
		Memory: fakeMemorySessions{
			sessions: map[string]*session.Session{
				"session-1": memoryQuerySession("session-1"),
			},
		},
	}

	records, err := service.ListMessages(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Content != "hello" {
		t.Fatalf("expected memory fallback messages, got %#v", records)
	}
}

func TestMessageQueryServiceUsesMemoryForMessagesAfterWhenStoreMissing(t *testing.T) {
	service := MessageQueryService{
		MemoryRecords: fakeMemoryMessageRecordMapper{},
		Memory: fakeMemorySessions{
			sessions: map[string]*session.Session{
				"session-1": {
					ID: "session-1",
					Messages: []domainmessage.Message{
						domainmessage.Text(domainmessage.RoleUser, "one"),
						domainmessage.Text(domainmessage.RoleAssistant, "two"),
					},
				},
			},
		},
	}

	records, truncated, err := service.ListMessagesAfter(context.Background(), "session-1", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(records) != 1 || records[0].Content != "one" {
		t.Fatalf("expected first memory record without full sync, got records=%#v truncated=%v", records, truncated)
	}
}

func TestMessageQueryServiceReturnsStoreSessionErrors(t *testing.T) {
	expected := errors.New("session missing")
	_, err := (MessageQueryService{
		Store: &fakeMessageQueryStore{sessionErr: expected},
	}).ListMessages(context.Background(), "session-1")

	if !errors.Is(err, expected) {
		t.Fatalf("expected store session error, got %v", err)
	}
}

type fakeMemorySessions struct {
	sessions map[string]*session.Session
}

type fakeMemoryMessageRecordMapper struct{}

func (fakeMemoryMessageRecordMapper) MemoryMessages(current *session.Session) []repository.MessageRecord {
	if current == nil {
		return nil
	}
	records := make([]repository.MessageRecord, 0, len(current.Messages))
	for index, message := range current.Messages {
		if message.Role == domainmessage.RoleSystem {
			continue
		}
		role := repository.RoleAssistant
		if message.Role == domainmessage.RoleUser {
			role = repository.RoleUser
		}
		records = append(records, repository.MessageRecord{
			ID:        fmt.Sprintf("message-%d", index+1),
			SessionID: current.ID,
			Role:      role,
			Content:   message.Text(),
		})
	}
	return records
}

func (m fakeMemorySessions) GetSession(sessionID string) (*session.Session, error) {
	current := m.sessions[sessionID]
	if current == nil {
		return nil, errors.New("session not found")
	}
	return current, nil
}

type fakeMessageQueryStore struct {
	checkedSession bool
	sessionErr     error
	messages       []repository.MessageRecord
	meta           repository.MessageHistoryMeta
	after          []repository.MessageRecord
	truncated      bool
}

func (s *fakeMessageQueryStore) GetSession(ctx context.Context, sessionID string) (repository.SessionRecord, error) {
	s.checkedSession = true
	if s.sessionErr != nil {
		return repository.SessionRecord{}, s.sessionErr
	}
	return repository.SessionRecord{ID: sessionID}, nil
}

func (s *fakeMessageQueryStore) ListMessages(ctx context.Context, sessionID string) ([]repository.MessageRecord, error) {
	return s.messages, nil
}

func (s *fakeMessageQueryStore) GetMessageHistoryMeta(ctx context.Context, sessionID string) (repository.MessageHistoryMeta, error) {
	return s.meta, nil
}

func (s *fakeMessageQueryStore) ListMessagesAfter(ctx context.Context, sessionID string, afterMessageID string, limit int) ([]repository.MessageRecord, bool, error) {
	return s.after, s.truncated, nil
}

func memoryQuerySession(sessionID string) *session.Session {
	return &session.Session{
		ID: sessionID,
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleUser, "hello"),
		},
	}
}
