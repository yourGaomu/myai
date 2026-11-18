package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	shortlinkssource "myai/core/adapter/documentsource/shortlinks"
	filesadapter "myai/core/adapter/files/shortlinks"
	documentcommand "myai/core/application/knowledge/document/command"
	toolcommand "myai/core/application/tool/command"
	toolservice "myai/core/application/tool/service"
	domainknowledge "myai/core/domain/knowledge"
	"myai/core/mcp"
	"myai/core/session"
	"myai/core/tool"
	"myai/core/tool/local"
)

func TestMCPFileShareReadAndKnowledgeIngest(t *testing.T) {
	ctx := context.Background()
	const text = "# MCP guide\nA small shared document."
	const assetID = "ast_guide"
	var mu sync.Mutex
	var uploaded []byte
	var fileName, contentType, key string
	var size int64
	var completed bool
	var calls []string
	objectCalls := 0
	object := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		objectCalls++
		for _, header := range []string{"X-Integration-MCP", "Mcp-Session-Id", "Authorization", "Cookie"} {
			if r.Header.Get(header) != "" {
				t.Errorf("object request received MCP header %s", header)
			}
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(uploaded)
	}))
	defer object.Close()
	asset := func() map[string]any {
		checksum := sha256.Sum256(uploaded)
		return map[string]any{
			"assetId": assetID, "businessId": "knowledge", "fileName": fileName, "contentType": contentType,
			"sizeBytes": size, "checksumSha256": hex.EncodeToString(checksum[:]), "createdAt": time.Now().UTC(),
		}
	}
	uploadSession := func() map[string]any {
		parts := []int{}
		if len(uploaded) > 0 {
			parts = append(parts, 1)
		}
		result := map[string]any{
			"uploadSessionId": "upl_guide", "idempotencyKey": key, "status": "uploading",
			"fileName": fileName, "businessId": "knowledge", "path": "uploads", "sizeBytes": size,
			"partSizeBytes": 1024, "partCount": 1, "receivedParts": parts, "receivedBytes": len(uploaded),
			"expiresAt": time.Now().Add(time.Hour),
		}
		if completed {
			result["status"], result["asset"] = "completed", asset()
		}
		return result
	}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var request struct {
			ID     int64  `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string                     `json:"name"`
				Arguments map[string]json.RawMessage `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode MCP request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		var result any
		switch request.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "composition-test")
			result = map[string]any{"protocolVersion": "2025-03-26"}
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
			return
		case "tools/list":
			var infos []mcp.ToolInfo
			for _, name := range []string{"begin_asset_upload", "upload_asset_part", "get_asset_upload_status", "complete_asset_upload", "create_asset_shortlink", "get_asset_download_url"} {
				infos = append(infos, mcp.ToolInfo{Name: name})
			}
			result = map[string]any{"tools": infos}
		case "tools/call":
			if r.Header.Get("Mcp-Session-Id") != "composition-test" || r.Header.Get("X-Integration-MCP") != "composition" {
				t.Error("managed MCP request lost its configured headers or session")
			}
			name, args := request.Params.Name, request.Params.Arguments
			calls = append(calls, name)
			var structured any
			switch name {
			case "begin_asset_upload":
				_ = json.Unmarshal(args["fileName"], &fileName)
				_ = json.Unmarshal(args["contentType"], &contentType)
				_ = json.Unmarshal(args["sizeBytes"], &size)
				_ = json.Unmarshal(args["idempotencyKey"], &key)
				structured = uploadSession()
			case "upload_asset_part":
				var encoded string
				_ = json.Unmarshal(args["contentBase64"], &encoded)
				var err error
				uploaded, err = base64.StdEncoding.DecodeString(encoded)
				if err != nil {
					t.Errorf("decode upload part: %v", err)
				}
				structured = uploadSession()
			case "complete_asset_upload":
				completed = true
				structured = uploadSession()
			case "create_asset_shortlink":
				if string(args["assetId"]) != `"`+assetID+`"` {
					t.Errorf("publication did not use uploaded asset ID: %#v", args)
				}
				structured = map[string]any{"asset": asset(), "shortUrl": "https://share.example/guide", "code": "guide", "maxVisits": 3, "expiresAt": time.Now().Add(time.Hour)}
			case "get_asset_download_url":
				if len(args) != 1 || string(args["assetId"]) != `"`+assetID+`"` {
					t.Errorf("download did not select only by asset ID: %#v", args)
				}
				structured = map[string]any{"asset": asset(), "downloadUrl": object.URL + "/guide?signature=local-test", "expiresAt": time.Now().Add(time.Hour)}
			default:
				t.Errorf("unexpected MCP file operation %q", name)
			}
			result = map[string]any{"structuredContent": structured, "content": []any{map[string]any{"type": "text", "text": "This free text is not file metadata."}}}
		default:
			t.Errorf("unexpected MCP method %q", request.Method)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}); err != nil {
			t.Errorf("encode MCP response: %v", err)
		}
	}))
	defer backend.Close()

	// Construct before the manager to exercise the application's lazy provider.
	var manager *mcp.Manager
	files := filesadapter.New(func() *mcp.Manager { return manager })
	manager = mcp.NewManager(mcp.Config{Servers: []mcp.ServerConfig{{
		Name: "short_links", URL: backend.URL + "/api/mcp", Required: true, TimeoutSeconds: 5,
		Headers: map[string]string{"X-Integration-MCP": "composition"},
	}}})
	defer manager.Close()
	if err := manager.RegisterAll(ctx, tool.NewRegisterTools()); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "guide.md"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	share := local.NewShareFileToolWithWorkspaceAndFiles(workspace, files)
	permission := toolcommand.Permission{
		Name: share.Name(), Arguments: `{"path":"guide.md"}`, Permission: share.Permission(),
		Mode: session.PermissionModeFull, WorkspaceRoot: workspace, ExplicitApproval: share.RequiresExplicitApproval(),
	}
	if decision := (toolservice.PermissionService{}).Allow(permission); decision.Allowed {
		t.Fatal("MCP sharing bypassed publication consent")
	}
	permission.Ask = func(toolcommand.PermissionRequest) bool { return true }
	if decision := (toolservice.PermissionService{}).Allow(permission); !decision.Allowed {
		t.Fatalf("explicit share consent rejected: %s", decision.Message)
	}
	output, err := share.Call(ctx, []byte(`{"path":"guide.md","title":"Guide","max_visits":3}`))
	if err != nil {
		t.Fatal(err)
	}
	var shared struct {
		AssetID    string `json:"asset_id"`
		BusinessID string `json:"business_id"`
		ShortURL   string `json:"short_url"`
		MaxVisits  int64  `json:"max_visits"`
	}
	if err := json.Unmarshal([]byte(output.Content), &shared); err != nil {
		t.Fatal(err)
	}
	if shared.AssetID != assetID || shared.BusinessID != "knowledge" || shared.ShortURL != "https://share.example/guide" || shared.MaxVisits != 3 {
		t.Fatalf("canonical share result = %s", output.Content)
	}
	readArgs, _ := json.Marshal(map[string]string{"asset_id": shared.AssetID})
	readOutput, err := local.NewReadAssetToolWithFiles(files).Call(ctx, readArgs)
	if err != nil {
		t.Fatal(err)
	}
	var read struct {
		AssetID string `json:"asset_id"`
		Parsed  struct {
			Text      string `json:"text"`
			Supported bool   `json:"supported"`
		} `json:"parsed"`
	}
	if err := json.Unmarshal([]byte(readOutput.Content), &read); err != nil {
		t.Fatal(err)
	}
	if read.AssetID != assetID || !read.Parsed.Supported || read.Parsed.Text != text || strings.Contains(readOutput.Content, "signature=") {
		t.Fatalf("read_asset result = %s", readOutput.Content)
	}
	source := shortlinkssource.Source{Client: files}
	content, err := source.Read(ctx, domainknowledge.DocumentSourceRequest{AssetID: shared.AssetID, MaxBytes: 1024})
	if err != nil || string(content.Data) != text {
		t.Fatalf("document source = %#v, %v", content, err)
	}
	fixtures := newDocumentFixtures()
	fixtures.service.Sources = source
	ingested, err := fixtures.service.Ingest(ctx, documentcommand.Ingest{KnowledgeBaseID: "knowledge-1", AssetID: shared.AssetID})
	if err != nil {
		t.Fatal(err)
	}
	if ingested.Document.FileName != "guide.md" || string(fixtures.objects.data) != text || fixtures.indexing.submitCalls != 1 {
		t.Fatalf("knowledge ingestion lost original bytes or indexing: %#v data=%q", ingested, fixtures.objects.data)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{"begin_asset_upload", "upload_asset_part", "complete_asset_upload", "create_asset_shortlink", "get_asset_download_url", "get_asset_download_url", "get_asset_download_url"}
	if !reflect.DeepEqual(calls, want) || objectCalls != 3 || string(uploaded) != text || size != int64(len(text)) || key == "" {
		t.Fatalf("unexpected MCP composition: calls=%v objectCalls=%d uploaded=%q size=%d", calls, objectCalls, uploaded, size)
	}
}
