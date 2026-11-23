package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	appconfig "myai/core/config"
	"myai/core/mcp"
	"myai/core/remote/agent"
	"myai/core/remote/protocol"
	"myai/core/tool"
	"myai/core/trust"

	"github.com/gorilla/websocket"
)

type mcpTrustIntegrationFacade struct {
	mcpIntegrationFacade
	workspace  string
	configPath string
}

func (f mcpTrustIntegrationFacade) MCPTrustWorkspace() string { return f.workspace }

func (f mcpTrustIntegrationFacade) config() (mcp.Config, error) {
	properties, err := (appconfig.ViperLoader{ConfigFile: f.configPath}).LoadMCP()
	return (appconfig.Mapper{}).MCPConfig(f.workspace, properties), err
}

func (f mcpTrustIntegrationFacade) MCPTrustCandidates(allowed func(string) bool) ([]mcp.MobileTrustCandidate, error) {
	config, err := f.config()
	if err != nil {
		return nil, err
	}
	return mcp.MobileTrustCandidates(config, f.workspace, allowed), nil
}

func (f mcpTrustIntegrationFacade) MCPTrustRuntime(name, digest string) string {
	return f.manager.MobileTrustRuntime(name, digest)
}

func (f mcpTrustIntegrationFacade) ReloadMCP(ctx context.Context) error {
	config, err := f.config()
	if err != nil {
		return err
	}
	return f.manager.Reload(ctx, config, f.registry)
}

func TestMCPTrustThroughPairedAgentAndRelay(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("APPDATA", configHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	t.Setenv("SHORT_LINKS_API_KEY", "")
	var upstreamCalls atomic.Int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		var req struct {
			ID     json.RawMessage
			Method string
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		result := map[string]any{}
		switch req.Method {
		case "initialize":
			result["protocolVersion"] = "2025-06-18"
			result["capabilities"] = map[string]any{"tools": struct{}{}}
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
			return
		case "tools/list":
			result["tools"] = []any{}
		default:
			t.Errorf("unexpected upstream method %s", req.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	defer backend.Close()
	endpoint := backend.URL + "/api/mcp"
	t.Setenv("SHORT_LINKS_MCP_URL", endpoint)
	workspace := t.TempDir()
	facade := mcpTrustIntegrationFacade{
		mcpIntegrationFacade: mcpIntegrationFacade{manager: mcp.NewManager(mcp.Config{}), registry: tool.NewRegisterTools()},
		workspace:            workspace, configPath: workspace + "/application.yaml",
	}
	defer facade.manager.Close()
	configFile := func(body string) {
		t.Helper()
		if err := os.WriteFile(facade.configPath, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	configFile("mcp: {}\n")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := facade.ReloadMCP(ctx); err != nil {
		t.Fatal(err)
	}

	relay := newTestServer()
	server := httptest.NewServer(relay.routes())
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	remoteAgent := agent.New(agent.Config{ServerURL: wsURL + "/ws/agent", RelayToken: "test-agent-token", UserID: "local", DeviceID: "pc-local", BindingCode: "123456"},
		mcpIntegrationChat{}, mcpIntegrationFiles{}, mcpIntegrationChanges{}, nil, nil, nil, nil, nil, nil, nil).WithMCPManager(facade)
	done := make(chan error, 1)
	go func() { done <- remoteAgent.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Agent did not stop")
		}
	}()
	for relay.getAgentByBindCode("123456") == nil {
		select {
		case <-time.After(10 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal("Agent did not register")
		}
	}
	managerToken := pairTestClient(t, server, "123456")
	// Pair codes are one-use, but a second active authorization can exist.
	ordinaryToken, _, err := relay.authorizeClient("local", "pc-local", "ordinary fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	admins, err := trust.DefaultMobileMCPStore()
	if err != nil {
		t.Fatal(err)
	}
	scope := trust.MobileMCPScope{Workspace: workspace, UserID: "local", DeviceID: "pc-local"}
	managerID := clientTokenHash(managerToken)
	if err := admins.Grant(scope, managerID, endpoint); err != nil {
		t.Fatal(err)
	}
	client := dialTestWebSocket(t, wsURL+"/ws/client")
	defer client.Close()
	ordinary := dialTestWebSocket(t, wsURL+"/ws/client")
	defer ordinary.Close()

	seq := 0
	send := func(socket *websocket.Conn, token string, kind, want protocol.MessageType, payload any) protocol.Message {
		t.Helper()
		seq++
		id := fmt.Sprintf("mobile-trust-fixture-%d", seq)
		message, err := protocol.NewMessage(kind, id, "local", "pc-local", "", payload)
		if err != nil {
			t.Fatal(err)
		}
		message.ClientToken = token
		writeTestMessage(t, socket, message)
		_ = socket.SetReadDeadline(time.Now().Add(5 * time.Second))
		for {
			var result protocol.Message
			if err := socket.ReadJSON(&result); err != nil {
				t.Fatal(err)
			}
			if result.Type == protocol.TypeHeartbeat {
				continue
			}
			if result.RequestID != id || result.Type != want || result.ClientToken != "" {
				t.Fatalf("unexpected trust result %+v", result)
			}
			return result
		}
	}
	request := func(socket *websocket.Conn, token, action, name, review string) protocol.MCPTrustResult {
		t.Helper()
		message := send(socket, token, protocol.TypeMCPTrust, protocol.TypeMCPTrustResult,
			protocol.MCPTrustRequest{Action: action, ServerName: name, ReviewID: review})
		result, err := protocol.DecodePayload[protocol.MCPTrustResult](message)
		if err != nil || result.Action != action || result.ServerName != name {
			t.Fatalf("invalid trust result: %+v %v", result, err)
		}
		return result
	}
	list := func() protocol.MCPTrustResult { return request(client, managerToken, "list", "", "") }
	review := func() string {
		t.Helper()
		result := request(client, managerToken, "review", "short_links", "")
		if result.Code != "" || result.ReviewID == "" || len(result.Services) != 1 || result.Services[0].Endpoint != endpoint {
			t.Fatalf("invalid review: %+v", result)
		}
		return result.ReviewID
	}
	approve := func(id string) protocol.MCPTrustResult {
		return request(client, managerToken, "approve", "short_links", id)
	}

	if result := request(ordinary, ordinaryToken, "list", "", ""); result.Code != protocol.MCPTrustDenied || len(result.Services) != 0 {
		t.Fatalf("ordinary pairing can view trust: %+v", result)
	}
	if result := list(); result.Code != "" || len(result.Services) != 1 || result.Services[0].Trusted || result.Services[0].Authentication != "headers_configured" {
		t.Fatalf("environment declaration not available for review: %+v", result)
	}
	stale := review()
	configFile(fmt.Sprintf("mcp:\n  servers:\n    - name: short_links\n      url: %s\n      disabled: true\n", endpoint))
	if result := list(); result.Code != "" || len(result.Services) != 0 {
		t.Fatalf("disabled YAML did not override environment: %+v", result)
	}
	if result := approve(stale); result.Code != protocol.MCPTrustReviewRequired {
		t.Fatalf("approved stale declaration: %+v", result)
	}
	// Replacing a local grant invalidates a previously issued review, even
	// when the same pairing is granted the same public endpoint once more.
	configFile(fmt.Sprintf("mcp:\n  servers:\n    - name: short_links\n      url: %s\n", endpoint))
	stale = review()
	if err := admins.Revoke(scope, managerID); err != nil {
		t.Fatal(err)
	}
	if result := approve(stale); result.Code != protocol.MCPTrustDenied {
		t.Fatalf("revoked grant still allowed: %+v", result)
	}
	if err := admins.Grant(scope, managerID, endpoint); err != nil {
		t.Fatal(err)
	}
	if result := approve(stale); result.Code != protocol.MCPTrustReviewRequired {
		t.Fatalf("old review survived regrant: %+v", result)
	}

	id := review()
	if result := request(ordinary, ordinaryToken, "approve", "short_links", id); result.Code != protocol.MCPTrustDenied {
		t.Fatalf("ordinary pairing could submit an approval: %+v", result)
	}
	if result := approve(id); result.Code != "" || len(result.Services) != 1 || !result.Services[0].Trusted || result.Services[0].Runtime != "not_loaded" {
		t.Fatalf("approval did not report saved separately from load: %+v", result)
	}
	if result := approve(id); result.Code != protocol.MCPTrustReviewRequired {
		t.Fatalf("approval replay succeeded: %+v", result)
	}
	if upstreamCalls.Load() != 0 {
		t.Fatal("review or approval contacted upstream")
	}
	config, err := facade.config()
	if err != nil {
		t.Fatal(err)
	}
	declaration, err := mcp.TrustRequest(config.Servers[0], workspace)
	if err != nil {
		t.Fatal(err)
	}
	store, err := trust.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	if trusted, err := store.Trusted(declaration); err != nil || !trusted {
		t.Fatalf("CLI declaration was not trusted: %v %v", trusted, err)
	}
	message := send(client, managerToken, protocol.TypeMCPReload, protocol.TypeMCPReloadResult, nil)
	if payload, err := protocol.DecodePayload[protocol.MCPReloadResultPayload](message); err != nil || !payload.Reloaded {
		t.Fatalf("manual MCP reload failed: %+v %v", payload, err)
	}
	if result := list(); len(result.Services) != 1 || result.Services[0].Runtime != "loaded" {
		t.Fatalf("loaded state: %+v", result)
	}
	configFile(fmt.Sprintf("mcp:\n  servers:\n    - name: short_links\n      url: %s\n      permission: read\n", endpoint))
	send(client, managerToken, protocol.TypeMCPReload, protocol.TypeError, nil)
	if result := list(); len(result.Services) != 1 || result.Services[0].Runtime != "older" || result.Services[0].Trusted {
		t.Fatalf("rollback state: %+v", result)
	}
}
