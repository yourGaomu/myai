package executor

import (
	"testing"
	"time"

	toolcommand "myai/core/application/tool/command"
	domainmessage "myai/core/domain/message"
)

func TestSharedAssetExtractorParsesTransportResult(t *testing.T) {
	createdAt := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	expiresAt := "2026-07-10T12:00:00Z"

	asset, ok := (SharedAssetExtractor{}).Extract(toolcommand.AssetExtraction{
		SessionID: "session-1",
		RequestID: "request-1",
		Call: domainmessage.ToolCall{
			ID:   "call-1",
			Name: "share_file",
		},
		Result:    `{"path":" ./file.txt ","short_url":" https://s/abc ","code":" abc ","file_name":" file.txt ","content_type":"text/plain","size":7,"expires_at":"` + expiresAt + `"}`,
		CreatedAt: createdAt,
	})
	if !ok {
		t.Fatal("expected shared asset")
	}
	if asset.SessionID != "session-1" || asset.ShortURL != "https://s/abc" || asset.ShortCode != "abc" {
		t.Fatalf("unexpected asset: %#v", asset)
	}
	if asset.ExpiresAt == nil || asset.ExpiresAt.Format(time.RFC3339) != expiresAt {
		t.Fatalf("unexpected expires at: %#v", asset.ExpiresAt)
	}
}

func TestSharedAssetExtractorRejectsNonShareFile(t *testing.T) {
	_, ok := (SharedAssetExtractor{}).Extract(toolcommand.AssetExtraction{
		Call:   domainmessage.ToolCall{Name: "read_file"},
		Result: `{"short_url":"https://s/abc"}`,
	})
	if ok {
		t.Fatal("expected non share_file result to be ignored")
	}
}
