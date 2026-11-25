package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	journal "myai/core/adapter/persistence/outbox/journal"
	files "myai/core/port/files"
	repository "myai/core/port/repository"
)

func (s *planRecoveryStore) FindSessionAttachment(_ context.Context, sessionID, assetID string) (repository.AssetRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, asset := range s.assets {
		if asset.SessionID == sessionID && asset.AssetID == assetID && asset.IsSessionAttachment() {
			return asset, nil
		}
	}
	return repository.AssetRecord{}, repository.ErrNotFound
}

func TestSessionAssetLookupClearRetainsPersistedAttachmentAndDiscardsPending(t *testing.T) {
	ctx := planRecoveryContext(t)
	h := newOutboxRecoveryHarness(t, journal.Options{})
	for _, id := range []string{"ast_persisted", "ast_pending"} {
		asset := files.Asset{AssetID: id, FileName: "attachment.txt", ContentType: "text/plain", Size: 5, CreatedAt: time.Now().UTC()}
		if err := h.chat.RecordUploadedAsset(ctx, h.sessionID, id, 0, asset); err != nil {
			t.Fatal(err)
		}
		if id == "ast_persisted" {
			h.async.drain()
		}
		if _, err := h.chat.LookupSessionAsset(ctx, h.sessionID, id); err != nil {
			t.Fatalf("fresh attachment %s unavailable: %v", id, err)
		}
	}
	if err := h.chat.ClearSession(ctx, h.sessionID); err != nil {
		t.Fatal(err)
	}
	h.async.drain()
	if _, err := h.chat.LookupSessionAsset(ctx, h.sessionID, "ast_pending"); !errors.Is(err, files.ErrSessionAssetUnavailable) {
		t.Fatalf("cleared pending attachment was admitted: %v", err)
	}
	if _, err := h.chat.LookupSessionAsset(ctx, h.sessionID, "ast_persisted"); err != nil {
		t.Fatalf("clear discarded a persisted attachment: %v", err)
	}
}

func TestSessionAssetLookupSeesPendingUploadAndPersistedAttachment(t *testing.T) {
	ctx := planRecoveryContext(t)
	h := newOutboxRecoveryHarness(t, journal.Options{})
	asset := files.Asset{AssetID: "ast_jpeg", FileName: "screenshot.jpg", ContentType: "image/jpeg", Size: 320, CreatedAt: time.Now().UTC()}
	if err := h.chat.RecordUploadedAsset(ctx, h.sessionID, "upload", 0, asset); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.FindSessionAttachment(ctx, h.sessionID, asset.AssetID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("test fixture already persisted the upload: %v", err)
	}
	assertAttachment := func() {
		t.Helper()
		attached, err := h.chat.LookupSessionAsset(ctx, h.sessionID, asset.AssetID)
		if err != nil || attached.AssetID != asset.AssetID || attached.Size != asset.Size {
			t.Fatalf("lookup did not find the upload receipt: %+v %v", attached, err)
		}
	}
	assertAttachment()
	h.async.drain()
	assertAttachment()

	if err := h.store.SaveSession(ctx, repository.SessionRecord{ID: "other-session"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.chat.LookupSessionAsset(ctx, "other-session", asset.AssetID); !errors.Is(err, files.ErrSessionAssetUnavailable) {
		t.Fatalf("another session was authorized: %v", err)
	}
	if err := h.store.SaveAsset(ctx, repository.AssetRecord{
		ID: "metadata-only", AssetID: asset.AssetID, SessionID: "other-session", ToolName: "mcp_short_links_get_asset_info", Size: asset.Size,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.chat.LookupSessionAsset(ctx, "other-session", asset.AssetID); !errors.Is(err, files.ErrSessionAssetUnavailable) {
		t.Fatalf("metadata-only result authorized an attachment: %v", err)
	}
	if err := h.chat.DeleteSession(ctx, h.sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.chat.LookupSessionAsset(ctx, h.sessionID, asset.AssetID); !errors.Is(err, files.ErrSessionAssetUnavailable) {
		t.Fatalf("deleted session was authorized: %v", err)
	}
}

func TestGeneratedAssetLookupResolvesItsSessionWorkspace(t *testing.T) {
	ctx := planRecoveryContext(t)
	h := newOutboxRecoveryHarness(t, journal.Options{})
	defaultWorkspace := t.TempDir()
	explicitWorkspace := t.TempDir()
	lookup := sessionAssetLookup(h.store, h.store, nil, defaultWorkspace)
	for _, test := range []struct {
		session repository.SessionRecord
		want    string
	}{
		{session: repository.SessionRecord{ID: "ordinary"}, want: defaultWorkspace},
		{session: repository.SessionRecord{ID: "explicit", WorkspaceRoot: explicitWorkspace}, want: explicitWorkspace},
		{session: repository.SessionRecord{ID: "isolated", WorkspaceSandboxID: "sandbox"}},
	} {
		const relative = ".myai/generated/preview.png"
		const assetID = "ast_preview"
		if err := h.store.SaveSession(ctx, test.session); err != nil {
			t.Fatal(err)
		}
		if err := h.store.SaveAsset(ctx, repository.AssetRecord{
			ID: test.session.ID, SessionID: test.session.ID, AssetID: assetID,
			ToolName: repository.AssetSourceGenerateImage, LocalPath: relative, Size: 42,
		}); err != nil {
			t.Fatal(err)
		}
		asset, err := lookup(ctx, test.session.ID, assetID)
		if err != nil || asset.WorkspaceRoot() != test.want || asset.LocalPath() != relative {
			t.Fatalf("%s workspace = %q, path = %q, err = %v", test.session.ID, asset.WorkspaceRoot(), asset.LocalPath(), err)
		}
	}
}
