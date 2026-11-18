package chat

import (
	"errors"
	"reflect"
	"testing"
	"time"

	journal "myai/core/adapter/persistence/outbox/journal"
	filesport "myai/core/port/files"
	outbox "myai/core/port/outbox"
)

func TestUploadedAssetUsesDurableQueueAndStableIdentity(t *testing.T) {
	ctx := planRecoveryContext(t)
	h := newOutboxRecoveryHarness(t, journal.Options{})
	asset := filesport.Asset{
		AssetID: "ast_uploaded", BusinessID: "business", FileName: "notes.txt",
		ContentType: "text/plain", Size: 5, Checksum: "checksum",
		CreatedAt: time.Date(2026, 11, 18, 1, 2, 3, 987654321, time.UTC),
	}
	for range 2 {
		if err := h.chat.RecordUploadedAsset(ctx, h.sessionID, "completion", 0, asset); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := h.journal.Pending(ctx)
	if err != nil || len(entries) != 1 || len(entries[0].Batch.Assets) != 1 {
		t.Fatalf("duplicate completion did not share one journal record: entries=%d error=%v", len(entries), err)
	}
	record := entries[0].Batch.Assets[0]
	if record.AssetID != asset.AssetID || record.Checksum != asset.Checksum || record.ShortURL != "" || record.RequestID != "completion" || record.CreatedAt.Nanosecond()%int(time.Millisecond) != 0 {
		t.Fatalf("upload-only metadata or stable timestamp lost: %+v", record)
	}
	if err := h.journal.Close(); err != nil {
		t.Fatal(err)
	}
	fresh := h.reopen(t)
	if err := fresh.chat.FlushPersistence(ctx); err != nil {
		t.Fatal(err)
	}
	items, err := fresh.store.ListAssets(ctx, h.sessionID, 10)
	if err != nil || len(items) != 1 || !reflect.DeepEqual(items[0], record) {
		t.Fatalf("recovery changed upload record: got=%+v error=%v", items, err)
	}
	if err := fresh.chat.RecordUploadedAsset(ctx, h.sessionID, "completion", 0, asset); err != nil {
		t.Fatal(err)
	}
	if err := fresh.chat.FlushPersistence(ctx); err != nil {
		t.Fatal(err)
	}
	items, err = fresh.store.ListAssets(ctx, h.sessionID, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("completion retry duplicated persisted asset: count=%d error=%v", len(items), err)
	}
}

func TestUploadedAssetRejectsBlockedOrMissingSession(t *testing.T) {
	ctx := planRecoveryContext(t)
	h := newOutboxRecoveryHarness(t, journal.Options{})
	asset := filesport.Asset{AssetID: "ast_uploaded", FileName: "notes.txt", Size: 1, CreatedAt: time.Now().UTC()}
	if err := h.chat.RecordUploadedAsset(ctx, "absent", "completion", 0, asset); err == nil {
		t.Fatal("missing session was initialized by an upload")
	}
	if err := h.chat.DeleteSession(ctx, h.sessionID); err != nil {
		t.Fatal(err)
	}
	if err := h.chat.RecordUploadedAsset(ctx, h.sessionID, "completion", 0, asset); err == nil {
		t.Fatal("deleted session accepted an uploaded asset")
	}
	entries, err := h.journal.Pending(ctx)
	if err != nil || len(entries) != 0 {
		t.Fatalf("blocked upload left replayable data: entries=%d error=%v", len(entries), err)
	}
}

func TestClearRejectsEarlierUploadAdmission(t *testing.T) {
	ctx := planRecoveryContext(t)
	h := newOutboxRecoveryHarness(t, journal.Options{})
	previous, err := h.chat.AdmitFileUpload(ctx, h.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.chat.ClearSession(ctx, h.sessionID); err != nil {
		t.Fatal(err)
	}
	asset := filesport.Asset{AssetID: "ast_uploaded", FileName: "notes.txt", Size: 1, CreatedAt: time.Now().UTC()}
	if err := h.chat.RecordUploadedAsset(ctx, h.sessionID, "old-completion", previous, asset); err == nil {
		t.Fatal("an upload admitted before clear was attached to the new session state")
	}
	entries, err := h.journal.Pending(ctx)
	if err != nil || len(entries) != 0 {
		t.Fatalf("stale completion entered the journal: count=%d error=%v", len(entries), err)
	}
	current, err := h.chat.AdmitFileUpload(ctx, h.sessionID)
	if err != nil || current == previous {
		t.Fatalf("new upload did not enter a new session epoch: error=%v", err)
	}
	if err := h.chat.RecordUploadedAsset(ctx, h.sessionID, "new-completion", current, asset); err != nil {
		t.Fatal(err)
	}
	h.async.drain()
	if err := h.chat.FlushPersistence(ctx); err != nil {
		t.Fatal(err)
	}
	items, err := h.store.ListAssets(ctx, h.sessionID, 10)
	if err != nil || len(items) != 1 || items[0].RequestID != "new-completion" {
		t.Fatalf("only the new admission should be persisted: assets=%+v error=%v", items, err)
	}
}

func TestUploadedAssetPropagatesJournalCapacityFailure(t *testing.T) {
	ctx := planRecoveryContext(t)
	h := newOutboxRecoveryHarness(t, journal.Options{})
	if err := h.journal.Close(); err != nil {
		t.Fatal(err)
	}
	asset := filesport.Asset{AssetID: "ast_uploaded", FileName: "notes.txt", Size: 1, CreatedAt: time.Now().UTC()}
	if err := h.chat.RecordUploadedAsset(ctx, h.sessionID, "completion", 0, asset); !errors.Is(err, outbox.ErrClosed) {
		t.Fatalf("journal rejection was swallowed: %v", err)
	}
}
