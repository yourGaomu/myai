package local

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"myai/core/port/files"
	toolruntime "myai/core/tool/runtimecontext"
)

type fakeAssetDownloader struct {
	files.Service
	response files.Download
	err      error
	request  files.DownloadRequest
}

func (f *fakeAssetDownloader) Download(_ context.Context, request files.DownloadRequest) (files.Download, error) {
	f.request = request
	return f.response, f.err
}

func TestReadAssetToolParsesTextAsset(t *testing.T) {
	downloader := &fakeAssetDownloader{
		response: files.Download{
			Asset: files.Asset{
				AssetID:     "asset-123",
				FileName:    "note.md",
				ContentType: "text/markdown",
				Size:        10,
			},
			Data: []byte("# Hello\nAI"),
		},
	}
	tool := NewReadAssetToolWithFiles(downloader)

	output, err := tool.Call(context.Background(), mustJSON(t, map[string]any{
		"asset_id":  " asset-123 ",
		"max_chars": 100,
	}))
	if err != nil {
		t.Fatalf("call read_asset: %v", err)
	}

	var result readAssetResult
	if err := json.Unmarshal([]byte(output.Content), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.AssetID != "asset-123" || !result.Parsed.Supported || result.Parsed.Text != "# Hello\nAI" {
		t.Fatalf("unexpected parsed asset: %#v", result)
	}
	if downloader.request.AssetID != "asset-123" || downloader.request.MaxBytes != defaultReadAssetMaxBytes {
		t.Fatalf("unexpected download request: %#v", downloader.request)
	}
}

func TestReadAssetToolRequiresAssetID(t *testing.T) {
	for _, raw := range []string{`{}`, `{"asset_id":" "}`, `{"url":"https://short.local/s/abc"}`, `{"code":"abc"}`} {
		t.Run(raw, func(t *testing.T) {
			tool := NewReadAssetToolWithFiles(&fakeAssetDownloader{})
			_, err := tool.Call(context.Background(), []byte(raw))
			if err == nil || !strings.Contains(err.Error(), "asset_id is required") {
				t.Fatalf("expected asset_id error, got %v", err)
			}
		})
	}
}

func TestReadAssetToolSchemaRequiresOnlyAssetID(t *testing.T) {
	schema := NewReadAssetToolWithFiles(nil).Schema().(map[string]any)
	if !reflect.DeepEqual(schema["required"], []string{"asset_id"}) {
		t.Fatalf("unexpected required fields: %#v", schema["required"])
	}
	properties := schema["properties"].(map[string]any)
	for _, legacy := range []string{"url", "code"} {
		if _, exists := properties[legacy]; exists {
			t.Fatalf("legacy download selector %q exposed", legacy)
		}
	}
}

func TestReadAssetToolRequiresConfiguredFiles(t *testing.T) {
	_, err := NewReadAssetToolWithFiles(nil).Call(context.Background(), []byte(`{"asset_id":"asset-123"}`))
	if err == nil || !strings.Contains(err.Error(), "MCP files service is not configured") {
		t.Fatalf("expected configured files error, got %v", err)
	}
}

func TestReadAssetToolReportsDownloadAndTextTruncation(t *testing.T) {
	for _, downloadTruncated := range []bool{false, true} {
		downloader := &fakeAssetDownloader{response: files.Download{
			Asset: files.Asset{AssetID: "asset-123", FileName: "note.txt", ContentType: "text/plain", Size: 20},
			Data:  []byte("你好世界"), Truncated: downloadTruncated,
		}}
		output, err := NewReadAssetToolWithFiles(downloader).Call(context.Background(), []byte(`{"asset_id":"asset-123","max_bytes":20,"max_chars":2}`))
		if err != nil {
			t.Fatal(err)
		}
		var result readAssetResult
		if err := json.Unmarshal([]byte(output.Content), &result); err != nil {
			t.Fatal(err)
		}
		if !output.Truncated || !result.Truncated || !result.Parsed.Truncated || result.Parsed.Text != "你好" {
			t.Fatalf("lost truncation: %#v", result)
		}
		if downloadTruncated && result.Parsed.Message == "" {
			t.Fatal("download truncation must be explained")
		}
		if downloader.request.MaxBytes != 20 {
			t.Fatalf("download limit not forwarded: %#v", downloader.request)
		}
	}
}

func TestReadAssetToolPropagatesDownloadError(t *testing.T) {
	want := errors.New("asset unavailable")
	_, err := NewReadAssetToolWithFiles(&fakeAssetDownloader{err: want}).Call(context.Background(), []byte(`{"asset_id":"asset-123"}`))
	if !errors.Is(err, want) {
		t.Fatalf("download error = %v, want %v", err, want)
	}
}

func TestScopedReadAssetChecksSessionAndReturnsImageReferenceWithoutFileBytes(t *testing.T) {
	asset := files.Asset{AssetID: "asset-123", FileName: "photo.jpg", ContentType: "image/jpeg", Size: 128, ShortURL: "https://example.com/private?sig=secret"}
	downloader := &fakeAssetDownloader{response: files.Download{Asset: asset, Data: []byte("image bytes")}}
	lookup := func(_ context.Context, sessionID, assetID string) (files.Asset, error) {
		if sessionID != "allowed" || assetID != asset.AssetID {
			return files.Asset{}, errors.New("fake rejection with private storage details")
		}
		return asset, nil
	}
	tool := NewReadAssetToolWithFiles(downloader, lookup)
	args := []byte(`{"asset_id":"asset-123"}`)
	wrongSession := toolruntime.WithExecution(t.Context(), toolruntime.Execution{SessionID: "wrong", ParentSessionID: "allowed"})
	rightSession := toolruntime.WithExecution(t.Context(), toolruntime.Execution{SessionID: "allowed"})
	for _, tc := range []struct {
		ctx  context.Context
		tool *ReadAssetTool
	}{
		{t.Context(), tool}, {wrongSession, tool}, {rightSession, NewReadAssetToolWithFiles(downloader, nil)},
	} {
		if _, err := tc.tool.Call(tc.ctx, args); !errors.Is(err, files.ErrSessionAssetUnavailable) {
			t.Fatalf("unauthorized call returned %v", err)
		}
	}
	output, err := tool.Call(rightSession, args)
	if err != nil {
		t.Fatal(err)
	}
	var result readAssetResult
	if err := json.Unmarshal([]byte(output.Content), &result); err != nil {
		t.Fatal(err)
	}
	if result.AssetID != asset.AssetID || !result.ImageReference || result.Parsed.Text != "" || result.Parsed.Supported || result.Parsed.Kind != "image" || result.Truncated {
		t.Fatalf("wrong image reference metadata: %+v", result)
	}
	if downloader.request.AssetID != "" || strings.Contains(output.Content, "head_hex") || strings.Contains(output.Content, "sig=secret") || strings.Contains(output.Content, "image bytes") {
		t.Fatal("image reference downloaded data or leaked private details")
	}
	output, err = tool.Call(rightSession, []byte(`{"asset_id":"asset-123","max_bytes":64}`))
	if err != nil || !output.Truncated || strings.Contains(output.Content, "image_reference") || !strings.Contains(output.Content, "Image content was not provided") || downloader.request.AssetID != "" {
		t.Fatalf("incorrect image limit handling: %+v %v", output, err)
	}
}

func TestScopedReadAssetRetainsTextAndRejectsMismatchedDownload(t *testing.T) {
	asset := files.Asset{AssetID: "note", FileName: "note.txt", ContentType: "text/plain", BusinessID: "demo", Size: 4}
	downloader := &fakeAssetDownloader{response: files.Download{Asset: asset, Data: []byte("note")}}
	tool := NewReadAssetToolWithFiles(downloader, func(_ context.Context, sessionID, assetID string) (files.Asset, error) {
		if sessionID != "allowed" || assetID != "note" {
			t.Fatal("lookup was not scoped to current session and requested asset")
		}
		return asset, nil
	})
	ctx := toolruntime.WithExecution(t.Context(), toolruntime.Execution{SessionID: "allowed"})
	args := []byte(`{"asset_id":"note"}`)
	output, err := tool.Call(ctx, args)
	if err != nil || !strings.Contains(output.Content, `"text": "note"`) || strings.Contains(output.Content, "image_reference") {
		t.Fatalf("text parsing regressed: %+v %v", output, err)
	}
	downloader.response.Asset.BusinessID = "other"
	if _, err := tool.Call(ctx, args); !errors.Is(err, files.ErrSessionAssetUnavailable) {
		t.Fatalf("mismatched download returned %v", err)
	}
}
