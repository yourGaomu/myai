package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
