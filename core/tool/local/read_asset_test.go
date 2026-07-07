package local

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"myai/core/asset"
)

type fakeAssetDownloader struct {
	response asset.DownloadAssetResponse
	err      error
	request  asset.DownloadAssetRequest
}

func (f *fakeAssetDownloader) DownloadAsset(ctx context.Context, request asset.DownloadAssetRequest) (asset.DownloadAssetResponse, error) {
	f.request = request
	return f.response, f.err
}

func TestReadAssetToolParsesTextAsset(t *testing.T) {
	downloader := &fakeAssetDownloader{
		response: asset.DownloadAssetResponse{
			URL:         "http://short.local/s/abc123",
			Code:        "abc123",
			FileName:    "note.md",
			ContentType: "text/markdown",
			Size:        11,
			Data:        []byte("# Hello\nAI"),
		},
	}
	tool := NewReadAssetToolWithDownloader(downloader)

	output, err := tool.Call(context.Background(), mustJSON(t, map[string]any{
		"url":       "http://short.local/s/abc123",
		"max_chars": 100,
	}))
	if err != nil {
		t.Fatalf("call read_asset: %v", err)
	}

	var result readAssetResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Code != "abc123" {
		t.Fatalf("unexpected code: %s", result.Code)
	}
	if !result.Parsed.Supported {
		t.Fatal("expected parsed text asset to be supported")
	}
	if result.Parsed.Text != "# Hello\nAI" {
		t.Fatalf("unexpected parsed text: %q", result.Parsed.Text)
	}
	if downloader.request.URL != "http://short.local/s/abc123" {
		t.Fatalf("unexpected downloader url: %s", downloader.request.URL)
	}
}

func TestReadAssetToolRequiresURLOrCode(t *testing.T) {
	tool := NewReadAssetToolWithDownloader(&fakeAssetDownloader{})

	_, err := tool.Call(context.Background(), mustJSON(t, map[string]any{}))
	if err == nil || !strings.Contains(err.Error(), "url or code is required") {
		t.Fatalf("expected url/code error, got %v", err)
	}
}

func TestReadAssetToolRequiresConfiguredDownloader(t *testing.T) {
	tool := NewReadAssetToolWithDownloader(nil)

	_, err := tool.Call(context.Background(), mustJSON(t, map[string]any{"code": "abc123"}))
	if err == nil || !strings.Contains(err.Error(), "asset short-link service is not configured") {
		t.Fatalf("expected configured downloader error, got %v", err)
	}
}
