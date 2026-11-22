package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func (f mcpIntegrationFacade) MCPServerStatuses() ([]mcp.ServerStatus, error) {
	return f.manager.ServerStatuses(), nil
}

func (f mcpIntegrationFacade) ReloadMCP(ctx context.Context) error {
	config := mcp.Config{Servers: []mcp.ServerConfig{{Name: "short_links", Disabled: true}}}
	return f.manager.Reload(ctx, config, f.registry)
}

func TestMCPListThroughPairedAgentAndRelay(t *testing.T) {
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
			result = map[string]any{"tools": []map[string]string{{"name": "create_url_shortlink"}, {"name": "begin_asset_upload"}}}
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

	manager := mcp.NewManager(mcp.Config{Servers: []mcp.ServerConfig{{
		Name: "short_links", URL: backend.URL + "/api/mcp", Trust: &trust.Policy{Allow: true}, TrustWorkspace: t.TempDir(),
	}}})
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

	request := func(kind, resultType protocol.MessageType, id string) protocol.Message {
		t.Helper()
		writeTestMessage(t, client, protocol.Message{Type: kind, RequestID: id, UserID: "local", DeviceID: "pc-local", ClientToken: credential})
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
	checkList := func(id string, want protocol.MCPServerSummary) {
		t.Helper()
		message := request(protocol.TypeMCPList, protocol.TypeMCPListResult, id)
		payload, err := protocol.DecodePayload[protocol.MCPListResultPayload](message)
		if err != nil || len(payload.Servers) != 1 || payload.Servers[0] != want {
			t.Fatalf("unexpected inventory %+v, err=%v", payload, err)
		}
	}
	checkList("list-before", protocol.MCPServerSummary{Name: "short_links", State: "loaded", InventoryToolCount: 2, ExposedToolCount: 1})
	message := request(protocol.TypeMCPReload, protocol.TypeMCPReloadResult, "reload")
	payload, err := protocol.DecodePayload[protocol.MCPReloadResultPayload](message)
	if err != nil || !payload.Reloaded {
		t.Fatalf("reload did not succeed, err=%v", err)
	}
	checkList("list-after", protocol.MCPServerSummary{Name: "short_links", State: "disabled"})
}
