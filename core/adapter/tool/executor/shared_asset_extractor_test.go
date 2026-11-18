package executor

import (
	"testing"
	"time"

	toolcommand "myai/core/application/tool/command"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
)

func TestSharedAssetExtractorParsesShareFileResult(t *testing.T) {
	createdAt := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	expiresAt := "2026-07-10T12:00:00Z"

	asset, ok := (SharedAssetExtractor{}).Extract(toolcommand.AssetExtraction{
		SessionID: "session-1",
		RequestID: "request-1",
		Call: domainmessage.ToolCall{
			ID:   "call-1",
			Name: "share_file",
		},
		Output:    domaintool.SuccessOutput(`{"asset_id":" asset-123 ","business_id":" business-123 ","path":" ./file.txt ","short_url":" https://s/abc ","code":" abc ","file_name":" file.txt ","content_type":"text/plain","size":7,"expires_at":"` + expiresAt + `","max_visits":5}`),
		CreatedAt: createdAt,
	})
	if !ok {
		t.Fatal("expected shared asset")
	}
	if asset.ID == "" || asset.SessionID != "session-1" || asset.RequestID != "request-1" || asset.ToolCallID != "call-1" || asset.ToolName != "share_file" || !asset.CreatedAt.Equal(createdAt) {
		t.Fatalf("unexpected asset identity: %#v", asset)
	}
	if asset.AssetID != "asset-123" || asset.BusinessID != "business-123" || asset.ShortURL != "https://s/abc" || asset.ShortCode != "abc" || asset.MaxVisits != 5 || asset.LocalPath != "./file.txt" {
		t.Fatalf("unexpected asset metadata: %#v", asset)
	}
	if asset.ExpiresAt == nil || asset.ExpiresAt.Format(time.RFC3339) != expiresAt {
		t.Fatalf("unexpected expires at: %#v", asset.ExpiresAt)
	}
}

func TestSharedAssetExtractorAcceptsTypedUploadWithoutShortURL(t *testing.T) {
	createdAt := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	expiresAt := createdAt.Add(time.Hour)
	typed := domaintool.SharedAsset{
		ID: "untrusted-event-id", AssetID: " asset-upload ", BusinessID: " business-upload ", Checksum: " sha256:test ",
		SessionID: "other-session", RequestID: "other-request", ToolCallID: "other-call", ToolName: "other-tool",
		FileName: "note.txt", ContentType: "text/plain", Size: 7, MaxVisits: 10, ExpiresAt: &expiresAt,
	}
	output := domaintool.SuccessOutput("human readable MCP content\n\n{\"unrelated\":true}")
	output.SharedAsset = &typed
	asset, ok := (SharedAssetExtractor{}).Extract(toolcommand.AssetExtraction{
		SessionID: "session-1", RequestID: "request-1",
		Call: domainmessage.ToolCall{ID: "call-1", Name: "mcp_short_links_upload_file"}, Output: output, CreatedAt: createdAt,
	})
	if !ok || asset.AssetID != "asset-upload" || asset.BusinessID != "business-upload" || asset.Checksum != "sha256:test" || asset.ShortURL != "" || asset.ShortCode != "" || asset.MaxVisits != 10 {
		t.Fatalf("typed upload metadata lost: %#v ok=%v", asset, ok)
	}
	if asset.ID == "" || asset.ID == typed.ID || asset.SessionID != "session-1" || asset.RequestID != "request-1" || asset.ToolCallID != "call-1" || asset.ToolName != "mcp_short_links_upload_file" || !asset.CreatedAt.Equal(createdAt) {
		t.Fatalf("typed metadata overrode current call identity: %#v", asset)
	}
	if asset.ExpiresAt == typed.ExpiresAt || !asset.ExpiresAt.Equal(expiresAt) {
		t.Fatal("typed expiry must be copied")
	}
	*asset.ExpiresAt = asset.ExpiresAt.Add(time.Hour)
	if !typed.ExpiresAt.Equal(expiresAt) {
		t.Fatal("extractor mutated adapter metadata")
	}
}

func TestSharedAssetExtractorRejectsUntrustedOrUnsuccessfulResults(t *testing.T) {
	canonical := `{"asset_id":"asset-123","short_url":"https://s/abc","code":"abc"}`
	for _, test := range []struct {
		name   string
		tool   string
		output domaintool.ToolOutput
	}{
		{name: "ordinary tool", tool: "read_file", output: domaintool.SuccessOutput(canonical)},
		{name: "generic MCP text", tool: "mcp_short_links_upload_file", output: domaintool.SuccessOutput(canonical)},
		{name: "legacy URL only", tool: "share_file", output: domaintool.SuccessOutput(`{"short_url":"https://s/abc","code":"abc"}`)},
		{name: "JSON upload without share URL", tool: "share_file", output: domaintool.SuccessOutput(`{"asset_id":"asset-123"}`)},
		{name: "missing asset ID", tool: "share_file", output: domaintool.SuccessOutput(`{"asset_id":" ","short_url":"https://s/abc"}`)},
		{name: "invalid JSON", tool: "share_file", output: domaintool.SuccessOutput("text\n" + canonical)},
		{name: "negative size", tool: "share_file", output: domaintool.SuccessOutput(`{"asset_id":"asset-123","short_url":"https://s/abc","size":-1}`)},
		{name: "failed", tool: "share_file", output: domaintool.ToolOutput{Content: canonical, Status: domaintool.ResultStatusFailed}},
		{name: "denied", tool: "share_file", output: domaintool.ToolOutput{Content: canonical, Status: domaintool.ResultStatusDenied}},
		{name: "skipped", tool: "share_file", output: domaintool.ToolOutput{Content: canonical, Status: domaintool.ResultStatusSkipped}},
		{name: "failed typed metadata", tool: "mcp_short_links_upload_file", output: domaintool.ToolOutput{Status: domaintool.ResultStatusFailed, SharedAsset: &domaintool.SharedAsset{AssetID: "asset-123"}}},
		{name: "empty typed identity", tool: "mcp_short_links_upload_file", output: domaintool.ToolOutput{Status: domaintool.ResultStatusSuccess, SharedAsset: &domaintool.SharedAsset{ShortURL: "https://s/abc"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if asset, ok := (SharedAssetExtractor{}).Extract(toolcommand.AssetExtraction{
				Call: domainmessage.ToolCall{Name: test.tool}, Output: test.output,
			}); ok {
				t.Fatalf("unexpected asset extracted: %#v", asset)
			}
		})
	}
}
