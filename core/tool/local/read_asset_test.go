package local

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"myai/core/port/files"
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
