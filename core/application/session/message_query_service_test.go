package sessionapp

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	chatmessagemapper "myai/core/adapter/persistence/chatmessage/mapper"
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

func TestMessageQueryServiceMergesStoreAndLiveMemoryMessages(t *testing.T) {
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

	if !store.checkedSession || len(records) != 2 || records[0].ID != "stored" || records[1].Content != "hello" {
		t.Fatalf("expected durable and live messages after session check, checked=%v records=%#v", store.checkedSession, records)
	}
}

func TestMergeMessageRecordsSortsMemoryOnlyRecordsByCursor(t *testing.T) {
	service := MessageQueryService{
		Store:         &fakeMessageQueryStore{messages: []repository.MessageRecord{{ID: "stored-20", Sequence: 20, CreatedAt: time.Unix(0, 20), Role: repository.RoleUser, Content: "middle"}}},
		Memory:        fakeMemorySessions{sessions: map[string]*session.Session{"session-1": {ID: "session-1"}}},
		MemoryRecords: orderedMemoryMessageRecordMapper{},
	}
	records, err := service.ListMessages(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || records[0].ID != "memory-10" || records[1].ID != "stored-20" || records[2].ID != "memory-30" {
		t.Fatalf("merged history is not cursor ordered: %#v", records)
	}
}

func TestMessageQueryServiceIncludesLiveTailInMetaAndDelta(t *testing.T) {
	store := &fakeMessageQueryStore{
		messages: []repository.MessageRecord{{ID: "stored", SessionID: "session-1", Role: repository.RoleUser, Content: "old"}},
	}
	service := MessageQueryService{
		Store:         store,
		MemoryRecords: fakeMemoryMessageRecordMapper{},
		Memory: fakeMemorySessions{sessions: map[string]*session.Session{
			"session-1": {ID: "session-1", Messages: []domainmessage.Message{
				domainmessage.Text(domainmessage.RoleUser, "old"),
				domainmessage.Text(domainmessage.RoleAssistant, "live answer"),
			}},
		}},
	}

	meta, err := service.HistoryMeta(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if meta.MessageCount != 2 || meta.LastMessageID == "stored" {
		t.Fatalf("expected live tail in history metadata, got %#v", meta)
	}

	delta, fullSync, err := service.ListMessagesAfter(context.Background(), "session-1", "stored", 10)
	if err != nil {
		t.Fatal(err)
	}
	if fullSync || len(delta) != 1 || delta[0].Content != "live answer" {
		t.Fatalf("expected live delta, fullSync=%v delta=%#v", fullSync, delta)
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

func TestMessageQueryServiceHidesSyntheticMessagesFromHistory(t *testing.T) {
	store := &fakeMessageQueryStore{messages: []repository.MessageRecord{
		{ID: "runtime-1", Role: repository.RoleSystem, SyntheticReason: "runtime_instruction", Content: "plan rules"},
		{ID: "subagent-1", Role: repository.RoleUser, SyntheticReason: "subagent_result", Content: "hidden report"},
		{ID: "user-1", Role: repository.RoleUser, Content: "hello"},
	}, meta: repository.MessageHistoryMeta{SessionID: "session-1", MessageCount: 1, LastMessageID: "user-1", HistoryVersion: 1}}
	service := MessageQueryService{Store: store}

	messages, err := service.ListMessages(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].ID != "user-1" {
		t.Fatalf("synthetic message leaked into history: %#v", messages)
	}
	meta, err := service.HistoryMeta(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if meta.MessageCount != 1 || meta.LastMessageID != "user-1" || meta.HistoryVersion != 1 {
		t.Fatalf("unexpected visible history metadata: %#v", meta)
	}
}

func TestMessageHistoryMetaUsesMonotonicRecordSequence(t *testing.T) {
	meta := MessageHistoryMetaFromRecords("session-1", []repository.MessageRecord{
		{ID: "message-1", Sequence: 100, CreatedAt: time.Unix(0, 100)},
		{ID: "message-2", Sequence: 120, CreatedAt: time.Unix(0, 120)},
	})
	if meta.HistoryVersion != 120 {
		t.Fatalf("expected sequence-based history version, got %#v", meta)
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

func TestMessageQueryServiceKeepsLiveCursorStableAcrossReads(t *testing.T) {
	current := session.NewFromState(session.InitialState{ID: "session-1"})
	current.AddUserMessage("hello")
	service := MessageQueryService{
		MemoryRecords: chatmessagemapper.Mapper{},
		Memory: fakeMemorySessions{sessions: map[string]*session.Session{
			"session-1": current,
		}},
	}

	first, err := service.ListMessages(context.Background(), "session-1")
	if err != nil || len(first) != 1 {
		t.Fatalf("first live read failed: records=%#v err=%v", first, err)
	}
	second, err := service.ListMessages(context.Background(), "session-1")
	if err != nil || len(second) != 1 || second[0].ID != first[0].ID {
		t.Fatalf("live cursor changed between reads: first=%#v second=%#v err=%v", first, second, err)
	}
	delta, fullSync, err := service.ListMessagesAfter(context.Background(), "session-1", first[0].ID, 10)
	if err != nil || fullSync || len(delta) != 0 {
		t.Fatalf("stable live cursor was rejected: delta=%#v fullSync=%v err=%v", delta, fullSync, err)
	}
	firstMeta, _ := service.HistoryMeta(context.Background(), "session-1")
	secondMeta, _ := service.HistoryMeta(context.Background(), "session-1")
	if firstMeta.HistoryVersion != secondMeta.HistoryVersion || firstMeta.LastMessageID != secondMeta.LastMessageID {
		t.Fatalf("live history metadata changed without a message: first=%#v second=%#v", firstMeta, secondMeta)
	}
}

func TestMessageQueryServiceUsesStoreDeltaWithoutLoadingFullHistory(t *testing.T) {
	store := &fakeMessageQueryStore{
		messages: []repository.MessageRecord{{ID: "full-history-should-not-be-read"}},
		after:    []repository.MessageRecord{{ID: "delta-1", Role: repository.RoleAssistant, Content: "new"}},
	}
	service := MessageQueryService{Store: store}

	items, fullSync, err := service.ListMessagesAfter(context.Background(), "session-1", "cursor-1", 20)
	if err != nil {
		t.Fatal(err)
	}
	if fullSync || len(items) != 1 || items[0].ID != "delta-1" {
		t.Fatalf("unexpected store delta: fullSync=%v items=%#v", fullSync, items)
	}
	if store.listMessagesCalls != 0 || store.listAfterCalls != 1 {
		t.Fatalf("expected direct store delta query, list=%d after=%d", store.listMessagesCalls, store.listAfterCalls)
	}
}

func TestMessageQueryServiceUsesStoreMetadataWithoutLoadingFullHistory(t *testing.T) {
	store := &fakeMessageQueryStore{
		messages: []repository.MessageRecord{{ID: "full-history-should-not-be-read"}},
		meta: repository.MessageHistoryMeta{
			SessionID: "session-1", MessageCount: 42, LastMessageID: "last", HistoryVersion: 9001,
		},
	}
	service := MessageQueryService{Store: store}

	meta, err := service.HistoryMeta(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if meta.MessageCount != 42 || meta.LastMessageID != "last" || meta.HistoryVersion != 9001 {
		t.Fatalf("unexpected store metadata: %#v", meta)
	}
	if store.listMessagesCalls != 0 || store.metaCalls != 1 {
		t.Fatalf("expected direct store metadata query, list=%d meta=%d", store.listMessagesCalls, store.metaCalls)
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

type orderedMemoryMessageRecordMapper struct{}

func (orderedMemoryMessageRecordMapper) MemoryMessages(*session.Session) []repository.MessageRecord {
	return []repository.MessageRecord{
		{ID: "memory-10", Sequence: 10, CreatedAt: time.Unix(0, 10), Role: repository.RoleUser, Content: "first"},
		{ID: "stored-20", Sequence: 20, CreatedAt: time.Unix(0, 20), Role: repository.RoleUser, Content: "middle"},
		{ID: "memory-30", Sequence: 30, CreatedAt: time.Unix(0, 30), Role: repository.RoleAssistant, Content: "last"},
	}
}

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
	checkedSession    bool
	listMessagesCalls int
	listAfterCalls    int
	metaCalls         int
	sessionErr        error
	messages          []repository.MessageRecord
	meta              repository.MessageHistoryMeta
	after             []repository.MessageRecord
	truncated         bool
}

func (s *fakeMessageQueryStore) GetSession(ctx context.Context, sessionID string) (repository.SessionRecord, error) {
	s.checkedSession = true
	if s.sessionErr != nil {
		return repository.SessionRecord{}, s.sessionErr
	}
	return repository.SessionRecord{ID: sessionID}, nil
}

func (s *fakeMessageQueryStore) ListMessages(ctx context.Context, sessionID string) ([]repository.MessageRecord, error) {
	s.listMessagesCalls++
	return s.messages, nil
}

func (s *fakeMessageQueryStore) GetMessageHistoryMeta(ctx context.Context, sessionID string) (repository.MessageHistoryMeta, error) {
	s.metaCalls++
	return s.meta, nil
}

func (s *fakeMessageQueryStore) ListMessagesAfter(ctx context.Context, sessionID string, afterMessageID string, limit int) ([]repository.MessageRecord, bool, error) {
	s.listAfterCalls++
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
