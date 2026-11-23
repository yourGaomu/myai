package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"myai/core/tool"
	tooldef "myai/core/tool/tool"
	"myai/core/trust"
)

func TestCatalogDetailUsesPublishedDiscoveryOnly(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request rpcRequest
		if json.NewDecoder(r.Body).Decode(&request) != nil || r.URL.Path == "/pending" {
			t.Error("unexpected MCP request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var result any
		switch request.Method {
		case "initialize":
			result = initializeResult{ProtocolVersion: defaultProtocolVersion}
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
			return
		case "tools/list":
			if r.URL.Path == "/discovery-failure" {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			infos := []ToolInfo{
				{Name: "begin_asset_upload"},
				{Name: "find.one", Title: "Lookup", Description: "Find an asset", InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","default":"do-not-copy","examples":["do-not-copy"]}},"required":["id"]}`), OutputSchema: json.RawMessage(`{"properties":{"found":{"type":["boolean","null"]}}}`)},
				{Name: "find_one", OutputSchema: json.RawMessage(`{"$ref":"do-not-copy"}`)},
			}
			for i := 0; i < 30; i++ {
				infos = append(infos, ToolInfo{Name: fmt.Sprintf("z%02d", i)})
			}
			result = listToolsResult{Tools: infos}
		default:
			t.Errorf("unexpected MCP method %s", request.Method)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()

	registry := tool.NewRegisterTools()
	workspace := t.TempDir()
	pending := trust.NewStore(filepath.Join(t.TempDir(), "trust.json"))
	loaded := ServerConfig{Name: "short_links", URL: server.URL, TrustWorkspace: workspace, Trust: &trust.Policy{Allow: true}}
	manager := NewManager(Config{Servers: []ServerConfig{
		loaded,
		{Name: "pending", URL: server.URL + "/pending", TrustWorkspace: workspace, Trust: &trust.Policy{Store: pending}},
		{Name: "discovery", URL: server.URL + "/discovery-failure", TrustWorkspace: workspace, Trust: &trust.Policy{Allow: true}},
	}})
	defer manager.Close()
	if err := manager.RegisterAll(context.Background(), registry); err != nil {
		t.Fatal(err)
	}
	before := calls.Load()
	statuses, revision := manager.CatalogStatuses()
	if statuses[1].Reason != ServerPendingTrust || statuses[2].Reason != ServerDiscoveryFailed {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}
	unavailable, err := manager.ServerDetail("pending", "", nil)
	if err != nil || unavailable.Tools == nil || len(unavailable.Tools) != 0 {
		t.Fatalf("pending service: %+v (%v)", unavailable, err)
	}
	page, err := manager.ServerDetail("short_links", "", func(alias string, _ tooldef.Permission) string {
		if alias == "mcp_short_links_find_one_2" {
			return "allowlist"
		}
		return ""
	})
	if err != nil || page.CatalogRevision != revision || len(page.Tools) != catalogPageSize || page.NextCursor == "" || !page.SessionEvaluated {
		t.Fatalf("unexpected first page: %+v (%v)", page, err)
	}
	hidden, visible, collision := page.Tools[0], page.Tools[1], page.Tools[2]
	if !hidden.HostOnly || hidden.Alias != "" || hidden.Eligibility != "host_only" || hidden.Input.State != "undeclared" {
		t.Fatalf("hidden tool: %+v", hidden)
	}
	if visible.Eligibility != "available" || visible.Title != "Lookup" || visible.Alias != "mcp_short_links_find_one" || visible.Input.Fields[0] != (SchemaField{Name: "id", Type: "string", Required: true}) || visible.Output.Fields[0].Type != "boolean | null" {
		t.Fatalf("visible tool: %+v", visible)
	}
	if collision.Alias != "mcp_short_links_find_one_2" || collision.Eligibility != "filtered" || collision.Reason != "allowlist" || collision.Output.State != "unsupported" || collision.ID == visible.ID {
		t.Fatalf("colliding tool: %+v", collision)
	}
	encoded, _ := json.Marshal(page)
	if strings.Contains(string(encoded), "do-not-copy") || strings.Contains(string(encoded), server.URL) {
		t.Fatalf("catalog exposes data outside its projection: %s", encoded)
	}
	page.Tools[1].Input.Fields[0].Name = "mutated"
	fresh, err := manager.ServerDetail("short_links", "", nil)
	if err != nil || fresh.Tools[1].Eligibility != "not_evaluated" || fresh.Tools[1].Input.Fields[0].Name != "id" {
		t.Fatalf("inspection mutated the catalog: %+v (%v)", fresh, err)
	}
	second, err := manager.ServerDetail("short_links", page.NextCursor, nil)
	if err != nil || len(second.Tools) != 8 || second.NextCursor != "" || calls.Load() != before {
		t.Fatalf("pagination contacted MCP or returned unexpected rows: %+v (%v)", second, err)
	}
	if _, err := manager.ServerDetail("pending", page.NextCursor, nil); !errors.Is(err, ErrCatalogInvalidCursor) {
		t.Fatalf("unexpected cross-service cursor error: %v", err)
	}
	bad := loaded
	bad.URL += "/discovery-failure"
	if err := manager.Reload(context.Background(), Config{Servers: []ServerConfig{bad}}, registry); err == nil {
		t.Fatal("expected reload failure")
	}
	if detail, err := manager.ServerDetail("short_links", page.NextCursor, nil); err != nil || detail.CatalogRevision != revision || len(detail.Tools) != 8 {
		t.Fatalf("failed reload changed catalog: %+v (%v)", detail, err)
	}
	if err := manager.Reload(context.Background(), Config{Servers: []ServerConfig{loaded}}, registry); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ServerDetail("short_links", page.NextCursor, nil); !errors.Is(err, ErrCatalogChanged) {
		t.Fatalf("unexpected old cursor error: %v", err)
	}
}

func TestCatalogSchemaBounds(t *testing.T) {
	properties := make(map[string]any)
	for i := 0; i < catalogSchemaFields+1; i++ {
		properties[fmt.Sprintf("%02d%s", i, strings.Repeat("界", 90))] = map[string]any{"type": []string{"string", "secret"}}
	}
	raw, _ := json.Marshal(map[string]any{"properties": properties})
	summary := summarizeSchema(raw)
	if summary.State != "fields" || len(summary.Fields) != catalogSchemaFields || !summary.Truncated || len(summary.Fields[0].Name) > 96 || summary.Fields[0].Type != "" {
		t.Fatalf("unexpected schema summary: %+v", summary)
	}
	if got := summarizeSchema([]byte(`{"type":"object","properties":{}}`)); got.State != "empty" {
		t.Fatalf("unexpected empty schema summary: %+v", got)
	}
	if got := summarizeSchema([]byte(strings.Repeat(" ", catalogSchemaBytes+1) + `{"type":"object"}`)); got.State != "unsupported" || !got.Truncated {
		t.Fatalf("unexpected oversized schema summary: %+v", got)
	}
}
