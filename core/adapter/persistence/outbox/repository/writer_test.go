package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	outbox "myai/core/port/outbox"
	repository "myai/core/port/repository"
)

type writerBackend struct {
	missing, deleted                           bool
	failure                                    error
	messageWrites, assetWrites, metadataWrites int
	lastMetadata                               repository.SessionRecord
}

func (b *writerBackend) GetSession(context.Context, string) (repository.SessionRecord, error) {
	if b.failure != nil {
		return repository.SessionRecord{}, b.failure
	}
	if b.missing {
		return repository.SessionRecord{}, repository.ErrNotFound
	}
	return repository.SessionRecord{ID: "session", Deleted: b.deleted}, nil
}
func (b *writerBackend) SaveMessage(context.Context, repository.MessageRecord) error {
	b.messageWrites++
	return nil
}
func (b *writerBackend) SaveAsset(context.Context, repository.AssetRecord) error {
	b.assetWrites++
	return nil
}
func (b *writerBackend) TouchSessionMessages(_ context.Context, record repository.SessionRecord, _ int64, _ bool) error {
	b.metadataWrites++
	b.lastMetadata = record
	return nil
}
func writerBatch() outbox.Batch {
	at := time.Now().UTC()
	return outbox.Batch{Version: outbox.Version, SessionID: "session", Messages: []repository.MessageRecord{{ID: "message", SessionID: "session", Role: repository.RoleUser, Content: "chat", Sequence: 1, CreatedAt: at}}, Metadata: &outbox.Metadata{CreatedAt: at, UpdatedAt: at}}
}
func TestWriterFailsBeforeWritesForMissingRestrictedAndDeletedSessions(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		missing, deleted, restricted bool
		want                         error
	}{
		{"restricted missing", true, false, true, repository.ErrNotFound},
		{"deleted", false, true, false, outbox.ErrBlocked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := &writerBackend{missing: tc.missing, deleted: tc.deleted}
			batch := writerBatch()
			batch.Metadata.RequiresExisting = tc.restricted
			writer := Writer{Messages: backend, Assets: backend, Metadata: backend, Sessions: backend}
			if err := writer.Apply(context.Background(), batch); !errors.Is(err, tc.want) {
				t.Fatalf("error=%v", err)
			}
			if backend.messageWrites+backend.assetWrites+backend.metadataWrites != 0 {
				t.Fatal("unsafe session was written before prerequisite check")
			}
		})
	}
}
func TestWriterMissingOrdinarySessionInitializesConservativeMetadata(t *testing.T) {
	backend := &writerBackend{missing: true}
	writer := Writer{Messages: backend, Assets: backend, Metadata: backend, Sessions: backend}
	if err := writer.Apply(context.Background(), writerBatch()); err != nil {
		t.Fatal(err)
	}
	if backend.messageWrites != 1 || backend.metadataWrites != 1 || backend.lastMetadata.PermissionMode != "ask" || backend.lastMetadata.AgentMode != "chat" || backend.lastMetadata.CurrentPlan != nil {
		t.Fatal("missing session did not use conservative initialization")
	}
}
func TestWriterBackendErrorDoesNotExposePayload(t *testing.T) {
	failure := errors.New("secret body or credential sentinel")
	backend := &writerBackend{failure: failure}
	err := (Writer{Messages: backend, Metadata: backend, Sessions: backend}).Apply(context.Background(), writerBatch())
	if !errors.Is(err, failure) || strings.Contains(err.Error(), "sentinel") {
		t.Fatalf("unsafe or unidentifiable error=%v", err)
	}
}
