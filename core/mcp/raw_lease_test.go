package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"myai/core/tool"
	tooldef "myai/core/tool/tool"
)

func rawLeaseServer(t *testing.T, marker string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     int64  `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		var result any
		switch request.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", marker)
			result = map[string]any{"protocolVersion": "2025-03-26"}
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
			return
		case "tools/list":
			result = map[string]any{"tools": []ToolInfo{{Name: "upload_asset_part"}, {Name: "create_asset_shortlink"}}}
		case "tools/call":
			if r.Header.Get("Mcp-Session-Id") != marker || request.Params.Name != "upload_asset_part" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			result = CallResult{StructuredContent: json.RawMessage(`{"session":"` + marker + `"}`)}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
}

func TestRawServerLeasePinsRuntimeUntilReleaseAcrossReloadAndClose(t *testing.T) {
	oldServer, newServer := rawLeaseServer(t, "old"), rawLeaseServer(t, "new")
	defer oldServer.Close()
	defer newServer.Close()
	registry := tool.NewRegisterTools()
	manager := NewManager(Config{Servers: []ServerConfig{{Name: "short_links", URL: oldServer.URL + "/api/mcp?credential=hidden", Required: true}}})
	if err := manager.RegisterAll(context.Background(), registry); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if _, err := registry.GetTool("mcp_short_links_upload_asset_part"); err == nil {
		t.Fatal("byte tool exposed to the model")
	}
	lease, err := manager.AcquireServer(context.Background(), "short_links")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if lease.Endpoint() != oldServer.URL || strings.Contains(lease.Endpoint(), "hidden") {
		t.Fatalf("endpoint leaked credentials: %s", lease.Endpoint())
	}
	oldClient := lease.runtime.client
	if _, err := lease.CallTool(context.Background(), "not-advertised", nil); err == nil {
		t.Fatal("unknown original tool accepted")
	}
	if err := manager.Reload(context.Background(), Config{Servers: []ServerConfig{{Name: "short_links", URL: newServer.URL, Required: true}}}, registry); err != nil {
		t.Fatal(err)
	}
	result, err := lease.CallTool(context.Background(), "upload_asset_part", json.RawMessage(`{}`))
	if err != nil || string(result.StructuredContent) != `{"session":"old"}` {
		t.Fatalf("old session = %s, %v", result.StructuredContent, err)
	}
	result, err = manager.CallTool(context.Background(), "short_links", "upload_asset_part", nil)
	if err != nil || string(result.StructuredContent) != `{"session":"new"}` {
		t.Fatalf("new session = %s, %v", result.StructuredContent, err)
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AcquireServer(context.Background(), "short_links"); err == nil {
		t.Fatal("closed manager issued a new lease")
	}
	if _, err := lease.CallTool(context.Background(), "upload_asset_part", nil); err != nil {
		t.Fatalf("close interrupted the pinned runtime: %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-oldClient.closed:
	default:
		t.Fatal("retired runtime was not closed after release")
	}
	if _, err := lease.CallTool(context.Background(), "upload_asset_part", nil); err == nil {
		t.Fatal("closed lease still callable")
	}
	_ = lease.Close()
}

func TestShortLinksExplicitApprovalAndByteToolVisibility(t *testing.T) {
	for _, name := range []string{"create_url_shortlink", "create_burn_link", "create_asset_shortlink", "upload_file_and_share"} {
		wrapped := NewToolWithName(nil, "short_links", ToolInfo{Name: name}, "exposed", tooldef.PermissionRead)
		if !wrapped.RequiresExplicitApproval() {
			t.Errorf("%s did not require approval", name)
		}
		other := NewToolWithName(nil, "other", ToolInfo{Name: name}, "exposed", tooldef.PermissionRead)
		if other.RequiresExplicitApproval() {
			t.Errorf("other server unexpectedly classified: %s", name)
		}
	}
	for _, name := range []string{"begin_asset_upload", "upload_asset_part", "get_asset_upload_status", "complete_asset_upload", "cancel_asset_upload", "upload_file_and_share", "get_asset_download_url"} {
		if !hiddenModelTool("short_links", name) || hiddenModelTool("other", name) {
			t.Errorf("incorrect visibility for %s", name)
		}
	}
	if hiddenModelTool("short_links", "create_asset_shortlink") {
		t.Fatal("existing asset publication must remain available with approval")
	}
}

func TestNormalizedStructuredFileOutputFailsClosed(t *testing.T) {
	asset := map[string]any{"assetId": "ast_one", "businessId": "canvas", "fileName": "test.txt", "contentType": "text/plain", "sizeBytes": 3, "createdAt": time.Now().UTC()}
	good := map[string]any{"asset": asset, "shortUrl": "https://short.example/r/one", "code": "one", "maxVisits": 1, "expiresAt": time.Now().Add(time.Hour)}
	encoded, _ := json.Marshal(good)
	result := CallResult{Content: []ContentItem{{Type: "text", Text: "secret https://signed.invalid/?token=do-not-copy"}}, StructuredContent: encoded}
	output := normalizedFileOutput(result, "create_asset_shortlink", json.RawMessage(`{"assetId":"ast_one"}`))
	if output.SharedAsset == nil || output.SharedAsset.AssetID != "ast_one" || output.Failed() || !json.Valid([]byte(output.Content)) || strings.Contains(output.Content, "secret") {
		t.Fatalf("canonical output = %+v", output)
	}
	for _, raw := range []string{`null`, `{}`, `{"asset":{"assetId":"ast_other"}}`, `{"asset":null,"shortUrl":"https://signed.invalid/?token=secret"}`} {
		result.StructuredContent = json.RawMessage(raw)
		output := normalizedFileOutput(result, "create_asset_shortlink", json.RawMessage(`{"assetId":"ast_one"}`))
		if !output.Failed() || output.SharedAsset != nil || strings.Contains(output.Content, "secret") {
			t.Errorf("accepted malformed file output: %+v", output)
		}
	}
	result.StructuredContent = encoded
	result.IsError = true
	if output := normalizedFileOutput(result, "create_asset_shortlink", json.RawMessage(`{"assetId":"ast_one"}`)); !output.Failed() || output.SharedAsset != nil {
		t.Fatal("error result populated SharedAsset")
	}
}
