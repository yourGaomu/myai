package local

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myai/core/port/files"
	tooldef "myai/core/tool/tool"
)

func TestShareFileToolRequiresExecutePermission(t *testing.T) {
	tool := NewShareFileToolWithWorkspaceAndFiles(".", nil)
	if permission := tool.Permission(); permission != tooldef.PermissionExecute {
		t.Fatalf("Permission() = %q, want %q", permission, tooldef.PermissionExecute)
	}
	if !tool.RequiresExplicitApproval() {
		t.Fatal("file publication must require explicit consent")
	}
}

type fakeAssetUploader struct {
	files.Service
	request files.ShareRequest
	content string
}

func (u *fakeAssetUploader) Share(_ context.Context, request files.ShareRequest) (files.Asset, error) {
	u.request = request
	data := new(strings.Builder)
	if _, err := io.Copy(data, request.Reader); err != nil {
		return files.Asset{}, err
	}
	u.content = data.String()
	return files.Asset{
		AssetID:     "asset-123",
		BusinessID:  "business-123",
		MaxVisits:   request.MaxVisits,
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
	tool := NewShareFileToolWithWorkspaceAndFiles(workspace, uploader)

	output, err := tool.Call(context.Background(), []byte(`{"path":"report.md","title":"Report","ttl_seconds":60,"max_visits":5}`))
	if err != nil {
		t.Fatalf("call share_file: %v", err)
	}

	var result shareFileResult
	if err := json.Unmarshal([]byte(output.Content), &result); err != nil {
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
	if result.AssetID != "asset-123" || result.BusinessID != "business-123" || result.MaxVisits != 5 {
		t.Fatalf("missing canonical asset metadata: %#v", result)
	}
	if uploader.request.Size != int64(len("# report")) || uploader.request.MaxVisits != 5 {
		t.Fatalf("unexpected share request size or visit limit: %#v", uploader.request)
	}
	if uploader.content != "# report" {
		t.Fatalf("unexpected upload content: %s", uploader.content)
	}
}

func TestShareFileVisitLimitMatchesMCPMeaning(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "report.txt"), []byte("report"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		args string
		want int64
	}{
		{"omitted_is_unlimited", `{"path":"report.txt"}`, -1},
		{"explicit_unlimited", `{"path":"report.txt","max_visits":-1}`, -1},
		{"explicit_disabled", `{"path":"report.txt","max_visits":0}`, 0},
		{"positive_limit", `{"path":"report.txt","max_visits":3}`, 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			uploader := &fakeAssetUploader{}
			output, err := NewShareFileToolWithWorkspaceAndFiles(workspace, uploader).Call(context.Background(), []byte(test.args))
			if err != nil {
				t.Fatal(err)
			}
			if uploader.request.MaxVisits != test.want {
				t.Fatalf("visit limit=%d, want %d", uploader.request.MaxVisits, test.want)
			}
			var result map[string]json.RawMessage
			if err := json.Unmarshal([]byte(output.Content), &result); err != nil {
				t.Fatal(err)
			}
			var visits int64
			if raw, exists := result["max_visits"]; !exists || json.Unmarshal(raw, &visits) != nil || visits != test.want {
				t.Fatal("canonical output lost the explicit visit policy")
			}
		})
	}
	if _, err := NewShareFileToolWithWorkspaceAndFiles(workspace, &fakeAssetUploader{}).Call(context.Background(), []byte(`{"path":"report.txt","max_visits":-2}`)); err == nil {
		t.Fatal("invalid negative visit limit was accepted")
	}
}

func TestShareFileToolRefusesSensitiveFile(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, ".env"), []byte("SECRET=1"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tool := NewShareFileToolWithWorkspaceAndFiles(workspace, &fakeAssetUploader{})
	if _, err := tool.Call(context.Background(), []byte(`{"path":".env"}`)); err == nil {
		t.Fatal("expected sensitive file error")
	}
}
