package shortlinkssource

import (
	"context"
	"errors"
	"strings"
	"testing"

	domainknowledge "myai/core/domain/knowledge"
	"myai/core/port/files"
)

type fileService struct {
	files.Service
	request  files.DownloadRequest
	response files.Download
	err      error
	calls    int
}

func (service *fileService) Download(_ context.Context, request files.DownloadRequest) (files.Download, error) {
	service.request = request
	service.calls++
	return service.response, service.err
}

func TestSourceReadsAssetIDAndCopiesDownload(t *testing.T) {
	service := &fileService{response: files.Download{
		Asset: files.Asset{AssetID: "asset-guide", FileName: "guide.md", ContentType: "text/markdown", Size: 5},
		Data:  []byte("guide"),
	}}
	content, err := (Source{Client: service}).Read(context.Background(), domainknowledge.DocumentSourceRequest{AssetID: " asset-guide ", MaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if service.request.AssetID != "asset-guide" || service.request.MaxBytes != 1024 {
		t.Fatalf("download request = %#v", service.request)
	}
	if content.FileName != "guide.md" || content.ContentType != "text/markdown" || content.Size != 5 || string(content.Data) != "guide" {
		t.Fatalf("source content = %#v", content)
	}
	service.response.Data[0] = 'X'
	if string(content.Data) != "guide" {
		t.Fatal("source returned the adapter's mutable byte slice")
	}
}

func TestSourceRejectsPartialOrOversizedDocuments(t *testing.T) {
	for _, test := range []struct {
		name      string
		truncated bool
		maxBytes  int64
	}{
		{name: "partial download", truncated: true, maxBytes: 10},
		{name: "oversized download", maxBytes: 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &fileService{response: files.Download{
				Asset: files.Asset{AssetID: "asset-guide", FileName: "guide.md"},
				Data:  []byte("guide"), Truncated: test.truncated,
			}}
			_, err := (Source{Client: service}).Read(context.Background(), domainknowledge.DocumentSourceRequest{AssetID: "asset-guide", MaxBytes: test.maxBytes})
			if err == nil || !strings.Contains(err.Error(), "exceeds the maximum ingest size") {
				t.Fatalf("expected ingest size error, got %v", err)
			}
		})
	}
}

func TestSourceValidatesInputAndPropagatesDownloadFailure(t *testing.T) {
	if _, err := (Source{}).Read(context.Background(), domainknowledge.DocumentSourceRequest{AssetID: "asset-guide"}); err == nil {
		t.Fatal("expected unconfigured source error")
	}
	want := errors.New("asset unavailable")
	service := &fileService{err: want}
	if _, err := (Source{Client: service}).Read(context.Background(), domainknowledge.DocumentSourceRequest{AssetID: " "}); err == nil || service.calls != 0 {
		t.Fatalf("blank asset ID reached download: calls=%d error=%v", service.calls, err)
	}
	if _, err := (Source{Client: service}).Read(context.Background(), domainknowledge.DocumentSourceRequest{AssetID: "asset-guide"}); !errors.Is(err, want) {
		t.Fatalf("source error = %v, want %v", err, want)
	}
}
