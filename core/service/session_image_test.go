package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	loadcommand "myai/core/application/session/load/command"
	"myai/core/port/files"
	"myai/core/session"
)

type localAssetSessionLoader struct{ workspace string }

func (l localAssetSessionLoader) Load(_ context.Context, id string) (*session.Session, error) {
	if id != "session-a" {
		return nil, files.ErrSessionAssetUnavailable
	}
	return &session.Session{ID: id, WorkspaceRoot: l.workspace}, nil
}
func (l localAssetSessionLoader) LoadCurrent(ctx context.Context, id string) (*session.Session, error) {
	return l.Load(ctx, id)
}
func (l localAssetSessionLoader) EnsureInMemory(ctx context.Context, command loadcommand.EnsureInMemory) (*session.Session, error) {
	return l.Load(ctx, command.SessionID)
}

func TestSessionImagePreviewReadsVerifiedLocalAssetWithoutMCP(t *testing.T) {
	workspace := t.TempDir()
	pixels := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	pixels.SetNRGBA(2, 2, color.NRGBA{R: 240, G: 20, B: 30, A: 180})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, pixels); err != nil {
		t.Fatal(err)
	}
	data := encoded.Bytes()
	if err := os.MkdirAll(filepath.Join(workspace, ".myai", "generated"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".myai", "generated", "test.png"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	asset := files.Asset{AssetID: "ast_local_test", FileName: "test.png", ContentType: "image/png", Size: int64(len(data)), Checksum: hex.EncodeToString(sum[:])}.WithLocalImage(".myai/generated/test.png", workspace)
	chat := NewChatService(ChatDependencies{SessionLoader: localAssetSessionLoader{workspace: workspace}})
	if err := chat.RegisterLocalSessionAsset(t.Context(), "session-a", asset); err != nil {
		t.Fatal(err)
	}
	preview, mime, width, height, err := chat.SessionImagePreview(t.Context(), "session-a", asset.AssetID)
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/png" || width != 8 || height != 6 || !bytes.Equal(preview, data) {
		t.Fatalf("unexpected preview %s %dx%d", mime, width, height)
	}
	if _, _, _, _, err := chat.SessionImagePreview(t.Context(), "session-b", asset.AssetID); err == nil {
		t.Fatal("cross-session asset lookup succeeded")
	}

	asset = asset.WithLocalImage("../outside.png", workspace)
	chat = NewChatService(ChatDependencies{SessionLoader: localAssetSessionLoader{workspace: workspace}})
	if err := chat.RegisterLocalSessionAsset(t.Context(), "session-a", asset); err == nil {
		t.Fatal("parent traversal was registered")
	}
}

// Only Download participates in the preview path.
type previewFilesStub struct {
	files.Service
	result files.Download
	err    error
	calls  int
}

func (stub *previewFilesStub) Download(_ context.Context, _ files.DownloadRequest) (files.Download, error) {
	stub.calls++
	return stub.result, stub.err
}

func TestSessionImagePreviewPrefersVerifiedGeneratedFileAfterUpload(t *testing.T) {
	workspace := t.TempDir()
	relative := ".myai/generated/test.png"
	path := filepath.Join(workspace, relative)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 8, 6))); err != nil {
		t.Fatal(err)
	}
	data := encoded.Bytes()
	checksum := sha256.Sum256(data)
	asset := files.Asset{
		AssetID: "ast_uploaded_png", BusinessID: "agent-uploads", FileName: "test.png",
		ContentType: "image/png", Size: int64(len(data)), Checksum: hex.EncodeToString(checksum[:]),
	}.WithLocalImage(relative, workspace)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	upstreamErr := errors.New("temporary object address is unreachable")
	objectStore := &previewFilesStub{err: upstreamErr}
	chat := NewChatService(ChatDependencies{
		AssetFiles: objectStore,
		LookupSessionAsset: func(_ context.Context, sessionID, assetID string) (files.Asset, error) {
			if sessionID != "session-a" || assetID != asset.AssetID {
				return files.Asset{}, files.ErrSessionAssetUnavailable
			}
			return asset, nil
		},
	})
	assertPreview := func() {
		t.Helper()
		preview, mime, width, height, err := chat.SessionImagePreview(t.Context(), "session-a", asset.AssetID)
		if err != nil || mime != "image/png" || width != 8 || height != 6 || !bytes.Equal(preview, data) {
			t.Fatalf("preview = %s %dx%d (%d bytes), error: %v", mime, width, height, len(preview), err)
		}
	}
	assertPreview()
	if objectStore.calls != 0 {
		t.Fatal("preview downloaded an already verified local image")
	}
	if _, _, _, _, err := chat.SessionImagePreview(t.Context(), "session-b", asset.AssetID); !errors.Is(err, files.ErrSessionAssetUnavailable) {
		t.Fatalf("cross-session preview error = %v", err)
	}
	if objectStore.calls != 0 {
		t.Fatal("unauthorized preview reached storage")
	}

	modified := bytes.Clone(data)
	modified[len(modified)-1] ^= 1
	if err := os.WriteFile(path, modified, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := chat.SessionImagePreview(t.Context(), "session-a", asset.AssetID); !errors.Is(err, ErrImagePreviewDownload) || !errors.Is(err, upstreamErr) {
		t.Fatalf("expected download error instead of bytes from the modified file: %v", err)
	}
	objectStore.err = nil
	objectStore.result = files.Download{Asset: asset, Data: data}
	assertPreview()
	if objectStore.calls != 2 {
		t.Fatalf("corrupt local image fallback made %d storage calls", objectStore.calls)
	}
	objectStore.result.Asset.BusinessID = "other-business"
	if _, _, _, _, err := chat.SessionImagePreview(t.Context(), "session-a", asset.AssetID); !errors.Is(err, ErrImagePreviewDownload) {
		t.Fatalf("preview accepted an object from another business: %v", err)
	}
}
