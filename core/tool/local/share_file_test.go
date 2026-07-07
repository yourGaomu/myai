package local

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myai/core/asset"
)

type fakeAssetUploader struct {
	request asset.UploadFileRequest
	content string
}

func (u *fakeAssetUploader) UploadFile(ctx context.Context, request asset.UploadFileRequest) (asset.UploadFileResponse, error) {
	u.request = request
	data := new(strings.Builder)
	if _, err := io.Copy(data, request.Reader); err != nil {
		return asset.UploadFileResponse{}, err
	}
	u.content = data.String()
	return asset.UploadFileResponse{
		Code:        "abc123",
		ShortURL:    "http://short.local/s/abc123",
		FileName:    request.FileName,
		ContentType: request.ContentType,
		Size:        int64(len(u.content)),
	}, nil
}

func TestShareFileTool(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "report.md")
	if err := os.WriteFile(path, []byte("# report"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	uploader := &fakeAssetUploader{}
	tool := NewShareFileToolWithWorkspaceAndUploader(workspace, uploader)

	output, err := tool.Call(context.Background(), []byte(`{"path":"report.md","title":"Report","ttl_seconds":60}`))
	if err != nil {
		t.Fatalf("call share_file: %v", err)
	}

	var result shareFileResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.ShortURL != "http://short.local/s/abc123" {
		t.Fatalf("unexpected short url: %s", result.ShortURL)
	}
	if uploader.request.FileName != "report.md" {
		t.Fatalf("unexpected uploaded file name: %s", uploader.request.FileName)
	}
	if uploader.request.Title != "Report" {
		t.Fatalf("unexpected title: %s", uploader.request.Title)
	}
	if uploader.request.TTLSeconds != 60 {
		t.Fatalf("unexpected ttl: %d", uploader.request.TTLSeconds)
	}
	if uploader.content != "# report" {
		t.Fatalf("unexpected upload content: %s", uploader.content)
	}
}

func TestShareFileToolRefusesSensitiveFile(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, ".env"), []byte("SECRET=1"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tool := NewShareFileToolWithWorkspaceAndUploader(workspace, &fakeAssetUploader{})
	if _, err := tool.Call(context.Background(), []byte(`{"path":".env"}`)); err == nil {
		t.Fatal("expected sensitive file error")
	}
}
