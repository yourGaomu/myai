package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"myai/core/tool"
	"myai/core/trust"
)

func TestServerStatusesTrackPublishedCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/unavailable" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		var request rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var result any
		switch request.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": defaultProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{}}}
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
			return
		case "tools/list":
			tools := []map[string]any{}
			if r.URL.Path == "/short-links" {
				tools = append(tools, map[string]any{"name": "create_url_shortlink"}, map[string]any{"name": "begin_asset_upload"})
			}
			result = map[string]any{"tools": tools}
		default:
			t.Errorf("unexpected MCP request %s", request.Method)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()

	ctx := context.Background()
	registry := tool.NewRegisterTools()
	manager := NewManager(Config{Servers: []ServerConfig{
		{Trust: &trust.Policy{Allow: true}, TrustWorkspace: t.TempDir(), Name: "short_links", URL: server.URL + "/short-links"},
		{Trust: &trust.Policy{Allow: true}, TrustWorkspace: t.TempDir(), Name: "empty", URL: server.URL + "/empty"},
		{Name: "off", Disabled: true},
		{Trust: &trust.Policy{Allow: true}, TrustWorkspace: t.TempDir(), Name: "offline", URL: server.URL + "/unavailable"},
	}})
	defer manager.Close()
	if err := manager.RegisterAll(ctx, registry); err != nil {
		t.Fatal(err)
	}
	want := []ServerStatus{
		{Name: "short_links", State: ServerLoaded, InventoryToolCount: 2, ExposedToolCount: 1},
		{Name: "empty", State: ServerLoaded},
		{Name: "off", State: ServerDisabled},
		{Name: "offline", State: ServerUnavailable, Reason: ServerStartFailed},
	}
	got := manager.ServerStatuses()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("statuses = %#v, want %#v", got, want)
	}
	got[0].Name = "changed in caller"
	if err := manager.Reload(ctx, Config{Servers: []ServerConfig{{Trust: &trust.Policy{Allow: true}, TrustWorkspace: t.TempDir(), Name: "bad candidate", URL: server.URL + "/unavailable"}}}, registry); err == nil {
		t.Fatal("expected reload failure")
	}
	if got := manager.ServerStatuses(); !reflect.DeepEqual(got, want) {
		t.Fatalf("failed reload changed statuses: %#v", got)
	}
	if err := manager.Reload(ctx, Config{}, registry); err != nil {
		t.Fatal(err)
	}
	if got := manager.ServerStatuses(); got == nil || len(got) != 0 {
		t.Fatalf("expected a non-nil, empty catalog, got %#v", got)
	}
}
