package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"myai/core/mcp"
	"myai/core/remote/agent"
	"myai/core/remote/protocol"
	"myai/core/tool"
	"myai/core/trust"
)

type mcpIntegrationChat struct{ agent.ChatFacade }

type mcpIntegrationFiles struct{ agent.WorkspaceFileFacade }

func (mcpIntegrationFiles) Root() string { return "test workspace" }

type mcpIntegrationChanges struct{ agent.WorkspaceChangeFacade }

func (mcpIntegrationChanges) Close() error { return nil }

type mcpIntegrationFacade struct {
	manager  *mcp.Manager
	registry *tool.RegisterTools
}

func (f mcpIntegrationFacade) MCPCatalogStatuses() ([]mcp.ServerStatus, uint64, error) {
	statuses, revision := f.manager.CatalogStatuses()
	return statuses, revision, nil
}

func (f mcpIntegrationFacade) MCPServerDetail(serverName, sessionID, cursor string) (mcp.ServerDetail, error) {
	// This fixture has no current ordinary session.
	return f.manager.ServerDetail(serverName, cursor, nil)
}

func (f mcpIntegrationFacade) ReloadMCP(ctx context.Context) error {
	config := mcp.Config{Servers: []mcp.ServerConfig{{Name: "short_links", Disabled: true}}}
	return f.manager.Reload(ctx, config, f.registry)
}

func TestMCPCatalogThroughPairedAgentAndRelay(t *testing.T) {
	var discoveries atomic.Int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var result any
		switch request.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{"tools": struct{}{}}}
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
			return
		case "tools/list":
			discoveries.Add(1)
			tools := []map[string]any{
				{"name": "create_url_shortlink", "title": "Create short link", "inputSchema": map[string]any{
					"type": "object", "properties": map[string]any{"url": map[string]any{"type": "string", "default": "never in the catalog"}}, "required": []string{"url"},
				}, "outputSchema": map[string]any{"type": "object", "properties": map[string]any{"short_url": map[string]any{"type": "string"}}}},
				{"name": "begin_asset_upload"},
			}
			for i := range 25 {
				tools = append(tools, map[string]any{"name": fmt.Sprintf("fixture_tool_%02d", i)})
			}
			result = map[string]any{"tools": tools}
		default:
			t.Errorf("unexpected MCP method %q", request.Method)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}); err != nil {
			t.Error(err)
		}
	}))
	defer backend.Close()

	manager := mcp.NewManager(mcp.Config{Servers: []mcp.ServerConfig{
		{Name: "short_links", URL: backend.URL + "/api/mcp", Trust: &trust.Policy{Allow: true}, TrustWorkspace: t.TempDir()},
		{Name: "restricted", URL: backend.URL + "/api/mcp", TrustWorkspace: t.TempDir()},
	}})
	defer manager.Close()
	registry := tool.NewRegisterTools()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := manager.RegisterAll(ctx, registry); err != nil {
		t.Fatal(err)
	}

	relay := newTestServer()
	server := httptest.NewServer(relay.routes())
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	remoteAgent := agent.New(agent.Config{
		ServerURL: wsURL + "/ws/agent", RelayToken: "test-agent-token",
		UserID: "local", DeviceID: "pc-local", BindingCode: "123456",
	}, mcpIntegrationChat{}, mcpIntegrationFiles{}, mcpIntegrationChanges{}, nil, nil, nil, nil, nil, nil, nil).
		WithMCPManager(mcpIntegrationFacade{manager: manager, registry: registry})
	done := make(chan error, 1)
	go func() { done <- remoteAgent.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("stopping Agent: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Agent did not stop")
		}
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for relay.getAgentByBindCode("123456") == nil {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("Agent did not register with Relay")
		}
	}
	credential := pairTestClient(t, server, "123456")
	client := dialTestWebSocket(t, wsURL+"/ws/client")
	defer client.Close()

	request := func(kind, resultType protocol.MessageType, id, sessionID string, payload any) protocol.Message {
		t.Helper()
		req, err := protocol.NewMessage(kind, id, "local", "pc-local", sessionID, payload)
		if err != nil {
			t.Fatal(err)
		}
		req.ClientToken = credential
		writeTestMessage(t, client, req)
		// Agent replies may race with the Relay's acknowledgement of forwarding.
		var result protocol.Message
		var acknowledged bool
		for range 2 {
			_ = client.SetReadDeadline(time.Now().Add(3 * time.Second))
			var message protocol.Message
			if err := client.ReadJSON(&message); err != nil {
				t.Fatal(err)
			}
			if message.RequestID != id {
				t.Fatalf("unexpected request id %q", message.RequestID)
			}
			switch message.Type {
			case resultType:
				result = message
			case protocol.TypeHeartbeat:
				acknowledged = true
			default:
				t.Fatalf("unexpected response type %s", message.Type)
			}
		}
		if result.Type != resultType || !acknowledged {
			t.Fatalf("expected %s and a Relay acknowledgement", resultType)
		}
		return result
	}
	checkList := func(id string, want ...protocol.MCPServerSummary) uint64 {
		t.Helper()
		message := request(protocol.TypeMCPList, protocol.TypeMCPListResult, id, "", nil)
		payload, err := protocol.DecodePayload[protocol.MCPListResultPayload](message)
		if err != nil || len(payload.Servers) != len(want) || payload.CatalogRevision == nil {
			t.Fatalf("unexpected inventory %+v, err=%v", payload, err)
		}
		for i, server := range payload.Servers {
			if server != want[i] {
				t.Fatalf("unexpected status %+v, want %+v", server, want[i])
			}
		}
		return *payload.CatalogRevision
	}
	checkDetail := func(id, session, cursor string) protocol.MCPDetailResultPayload {
		t.Helper()
		message := request(protocol.TypeMCPDetail, protocol.TypeMCPDetailResult, id, session, protocol.MCPDetailPayload{ServerName: "short_links", Cursor: cursor})
		payload, err := protocol.MCPDetailResult(message)
		if err != nil || payload.SessionEvaluated || message.SessionID != session || strings.Contains(string(message.Payload), "never in the catalog") {
			t.Fatalf("unexpected detail %+v, err=%v", payload, err)
		}
		return payload
	}

	loaded := protocol.MCPServerSummary{Name: "short_links", State: "loaded", InventoryToolCount: 27, ExposedToolCount: 26}
	restricted := protocol.MCPServerSummary{Name: "restricted", State: "unavailable", Reason: "pending_trust"}
	revision := checkList("list-before", loaded, restricted)
	pending := request(protocol.TypeMCPDetail, protocol.TypeMCPDetailResult, "detail-pending", "", protocol.MCPDetailPayload{ServerName: restricted.Name})
	pendingPayload, err := protocol.MCPDetailResult(pending)
	if err != nil || pendingPayload.Server != restricted || len(pendingPayload.Tools) != 0 || pendingPayload.CatalogRevision != revision {
		t.Fatalf("unexpected pending-trust detail: %+v (%v)", pendingPayload, err)
	}
	page := checkDetail("detail-first", "not-current", "")
	if page.Server != loaded || page.CatalogRevision != revision || len(page.Tools) != 25 || page.NextCursor == "" {
		t.Fatalf("unexpected first page: %+v", page)
	}
	byName := make(map[string]protocol.MCPToolDescriptor)
	for _, tool := range page.Tools {
		byName[tool.Name] = tool
	}
	exposed, hidden := byName["create_url_shortlink"], byName["begin_asset_upload"]
	if exposed.Eligibility != "not_evaluated" || exposed.Alias == "" || exposed.Title != "Create short link" ||
		len(exposed.Input.Fields) != 1 || !exposed.Input.Fields[0].Required || len(exposed.Output.Fields) != 1 ||
		exposed.Output.Fields[0].Name != "short_url" || !hidden.HostOnly || hidden.Eligibility != "host_only" {
		t.Fatalf("unexpected projection for exposed and hidden tools: %+v %+v", exposed, hidden)
	}
	second := checkDetail("detail-second", "", page.NextCursor)
	if second.CatalogRevision != revision || second.NextCursor != "" || len(second.Tools) != 2 || discoveries.Load() != 1 {
		t.Fatalf("unexpected second page or redundant tools/list: %+v", second)
	}

	message := request(protocol.TypeMCPReload, protocol.TypeMCPReloadResult, "reload", "", nil)
	payload, err := protocol.DecodePayload[protocol.MCPReloadResultPayload](message)
	if err != nil || !payload.Reloaded {
		t.Fatalf("reload did not succeed, err=%v", err)
	}
	disabled := protocol.MCPServerSummary{Name: "short_links", State: "disabled"}
	updated := checkList("list-after", disabled)
	if updated == revision {
		t.Fatal("successful reload did not advance the revision")
	}
	stale := request(protocol.TypeMCPDetail, protocol.TypeError, "detail-stale", "not-current", protocol.MCPDetailPayload{ServerName: "short_links", Cursor: page.NextCursor})
	failure, err := protocol.MCPDetailError(stale)
	if err != nil || failure.Code != protocol.MCPErrorCatalogChanged || failure.ServerName != "short_links" || stale.SessionID != "not-current" {
		t.Fatalf("unexpected stale response: %+v (%v)", stale, err)
	}
	fresh := checkDetail("detail-fresh", "", "")
	if fresh.Server != disabled || fresh.CatalogRevision != updated || len(fresh.Tools) != 0 || discoveries.Load() != 1 {
		t.Fatalf("unexpected disabled detail or redundant tools/list: %+v", fresh)
	}
}
