package sessionapp

import (
	"context"
	"errors"
	"testing"
	"time"

	memorysession "myai/core/adapter/session/memory"
	repository "myai/core/port/repository"
	"myai/core/session"
)

func TestSessionPersistenceServiceSavesCurrentMemoryState(t *testing.T) {
	memory := memorysession.NewStore("gpt-default")
	if err := memory.PutSessionWithOptions("session-1", "gpt-5", session.PermissionModeReadonly, 8, nil); err != nil {
		t.Fatal(err)
	}
	repository := &fakeSessionPersistenceRepository{getErr: repository.ErrNotFound}
	service := SessionPersistenceService{
		Memory:       memory,
		Sessions:     repository,
		DefaultModel: "gpt-default",
		Now:          fixedPersistenceTime,
	}

	if err := service.Save(context.Background(), SaveSessionCommand{
		SessionID: "session-1",
		Title:     "hello",
	}); err != nil {
		t.Fatal(err)
	}
	if repository.saved.ID != "session-1" || repository.saved.Model != "gpt-default" {
		t.Fatalf("unexpected saved identity: %#v", repository.saved)
	}
	if repository.saved.PermissionMode != string(session.PermissionModeReadonly) || repository.saved.ContextWindowK != 8 {
		t.Fatalf("expected memory state to be saved: %#v", repository.saved)
	}
	if repository.saved.Title != "hello" || !repository.saved.UpdatedAt.Equal(fixedPersistenceTime()) {
		t.Fatalf("unexpected saved metadata: %#v", repository.saved)
	}
}

func TestSessionPersistenceServicePreservesExistingMetadata(t *testing.T) {
	createdAt := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	repository := &fakeSessionPersistenceRepository{record: repository.SessionRecord{
		ID:             "session-1",
		Model:          "gpt-old",
		PermissionMode: string(session.PermissionModeFull),
		Title:          "existing title",
		CreatedAt:      createdAt,
	}}
	service := SessionPersistenceService{
		Sessions:     repository,
		DefaultModel: "gpt-default",
		Now:          fixedPersistenceTime,
	}

	if err := service.Save(context.Background(), SaveSessionCommand{SessionID: "session-1"}); err != nil {
		t.Fatal(err)
	}
	if repository.saved.Title != "existing title" || !repository.saved.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected existing metadata to be preserved: %#v", repository.saved)
	}
	if repository.saved.PermissionMode != string(session.PermissionModeFull) {
		t.Fatalf("expected existing state fallback: %#v", repository.saved)
	}
}

func TestSessionPersistenceServicePropagatesRepositoryError(t *testing.T) {
	expected := errors.New("database unavailable")
	service := SessionPersistenceService{
		Sessions: &fakeSessionPersistenceRepository{getErr: expected},
	}

	if err := service.Save(context.Background(), SaveSessionCommand{SessionID: "session-1"}); !errors.Is(err, expected) {
		t.Fatalf("expected repository error, got %v", err)
	}
}

type fakeSessionPersistenceRepository struct {
	record repository.SessionRecord
	getErr error
	saved  repository.SessionRecord
}

func (r *fakeSessionPersistenceRepository) GetSession(context.Context, string) (repository.SessionRecord, error) {
	if r.getErr != nil {
		return repository.SessionRecord{}, r.getErr
	}
	return r.record, nil
}

func (r *fakeSessionPersistenceRepository) SaveSession(_ context.Context, record repository.SessionRecord) error {
	r.saved = record
	return nil
}

func fixedPersistenceTime() time.Time {
	return time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
}
