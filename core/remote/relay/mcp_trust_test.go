package relay

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"myai/core/remote/protocol"
)

func mcpTrustFixture(t *testing.T) (s *Server, server *httptest.Server, agent, client *websocket.Conn, token string) {
	t.Helper()
	s = newTestServer()
	WithAgentCredentials(AgentCredential{UserID: "other", DeviceID: "workstation", Token: "other-agent-token"})(s)
	server = httptest.NewServer(s.routes())
	t.Cleanup(server.Close)
	base := "ws" + strings.TrimPrefix(server.URL, "http")
	agent = dialTestWebSocket(t, base+"/ws/agent")
	t.Cleanup(func() { _ = agent.Close() })
	writeAgentOnline(t, agent, "local", "pc-local", "123456")
	readTestMessage(t, agent, protocol.TypeHeartbeat)
	client = dialTestWebSocket(t, base+"/ws/client")
	t.Cleanup(func() { _ = client.Close() })
	token = pairTestClient(t, server, "123456")
	return
}

func postMCPTrustCheck(t *testing.T, server *httptest.Server, method, requestID, token, user, device string, extra map[string]string, want int) {
	t.Helper()
	body, err := json.Marshal(protocol.MCPTrustCheck{RequestID: requestID})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(method, server.URL+protocol.MCPTrustCheckPath, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set(protocol.HeaderAgentUserID, user)
	req.Header.Set(protocol.HeaderAgentDeviceID, device)
	for key, value := range extra {
		req.Header.Set(key, value)
	}
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != want || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected check response: %s, want %d", res.Status, want)
	}
}

func testMCPTrustResult(t *testing.T, client *websocket.Conn, request protocol.Message, action, service, code string) protocol.MCPTrustResult {
	t.Helper()
	reply := readTestMessage(t, client, protocol.TypeMCPTrustResult)
	var got protocol.MCPTrustResult
	if err := json.Unmarshal(reply.Payload, &got); err != nil {
		t.Fatal(err)
	}
	if reply.RequestID != request.RequestID || reply.SessionID != request.SessionID || reply.ClientToken != "" || got.Action != action || got.ServerName != service || got.Code != code {
		t.Fatalf("unexpected trust reply %+v: %+v", reply, got)
	}
	return got
}

func testMCPTrustService() protocol.MCPTrustService {
	return protocol.MCPTrustService{Name: "demo", Endpoint: "https://mcp.example/api", Permission: "read", Authentication: "none", Runtime: "not_loaded", Trusted: true}
}

func TestRelayMCPTrustSocketCorrelationAndCheck(t *testing.T) {
	s, server, agent, client, token := mcpTrustFixture(t)
	request := testMCPDetailMessage(t, protocol.TypeMCPTrust, "mobile-1", "exact-session", map[string]string{
		"action": "review", "server_name": "demo", "pairing_id": "spoofed", "connection_id": "spoofed", "role": "admin",
	})
	request.ClientToken = token

	for _, kind := range []protocol.MessageType{protocol.TypeMCPTrustInternal, protocol.TypeMCPTrustResult} {
		forged := request
		forged.Type = kind
		writeTestMessage(t, client, forged)
		readTestMessage(t, client, protocol.TypeError)
	}
	reserved := request
	reserved.RequestID = mcpTrustIDPrefix + uuid.NewString()
	writeTestMessage(t, client, reserved)
	readTestMessage(t, client, protocol.TypeError)

	writeTestMessage(t, client, request)
	forwarded := readTestMessage(t, agent, protocol.TypeMCPTrustInternal)
	readTestMessage(t, client, protocol.TypeHeartbeat)
	var internal protocol.MCPTrustInternal
	if err := json.Unmarshal(forwarded.Payload, &internal); err != nil {
		t.Fatal(err)
	}
	if !validMCPTrustAgentID(forwarded.RequestID) || forwarded.RequestID == request.RequestID || forwarded.SessionID != request.SessionID || forwarded.ClientToken != "" ||
		internal.Request.Action != "review" || internal.Request.ServerName != "demo" || internal.PairingID != clientTokenHash(token) ||
		internal.ConnectionID == "" || strings.Contains(string(forwarded.Payload), "spoofed") {
		t.Fatalf("invalid internal request: %+v / %+v", forwarded, internal)
	}

	postMCPTrustCheck(t, server, http.MethodGet, forwarded.RequestID, "test-agent-token", "local", "pc-local", nil, http.StatusMethodNotAllowed)
	postMCPTrustCheck(t, server, http.MethodOptions, forwarded.RequestID, "test-agent-token", "local", "pc-local", nil, http.StatusMethodNotAllowed)
	postMCPTrustCheck(t, server, http.MethodPost, forwarded.RequestID, token, "local", "pc-local", nil, http.StatusUnauthorized)
	postMCPTrustCheck(t, server, http.MethodPost, forwarded.RequestID, "other-agent-token", "other", "workstation", nil, http.StatusForbidden)
	postMCPTrustCheck(t, server, http.MethodPost, forwarded.RequestID, "test-agent-token", "local", "pc-local", map[string]string{"X-Forwarded-Proto": "https"}, http.StatusUnauthorized)
	postMCPTrustCheck(t, server, http.MethodPost, request.RequestID, "test-agent-token", "local", "pc-local", nil, http.StatusBadRequest)
	check := func(id string, want int) {
		postMCPTrustCheck(t, server, http.MethodPost, id, "test-agent-token", "local", "pc-local", nil, want)
	}
	check(forwarded.RequestID, http.StatusNoContent)
	check(forwarded.RequestID, http.StatusNoContent)

	other := dialTestWebSocket(t, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws/client")
	defer other.Close()
	writeTestMessage(t, other, request)
	testMCPTrustResult(t, other, request, "review", "demo", protocol.MCPTrustUnavailable)

	valid := protocol.MCPTrustResult{Action: "review", ServerName: "demo", ReviewID: uuid.NewString(), Services: []protocol.MCPTrustService{testMCPTrustService()}}
	reply := testMCPDetailMessage(t, protocol.TypeMCPTrustResult, forwarded.RequestID, request.SessionID, valid)
	bogus := reply
	bogus.RequestID = request.RequestID
	if err := s.forwardMCPTrust(s.getAgent(request.UserID, request.DeviceID).peer, bogus); err != nil || s.getClient(request.RequestID) == nil {
		t.Fatal("a reply with the mobile correlation ID settled the request")
	}
	bogus = reply
	bogus.SessionID = "different-session"
	if err := s.forwardMCPTrust(s.getAgent(request.UserID, request.DeviceID).peer, bogus); err != nil || s.getClient(request.RequestID) == nil {
		t.Fatal("a reply from the wrong session settled the request")
	}
	if err := s.forwardMCPTrust(new(peer), reply); err != nil || s.getClient(request.RequestID) == nil {
		t.Fatal("a reply from the wrong Agent socket settled the request")
	}
	reply.ClientToken = "unforwarded-agent-value"
	writeTestMessage(t, agent, reply)
	got := testMCPTrustResult(t, client, request, "review", "demo", "")
	if got.ReviewID != valid.ReviewID || len(got.Services) != 1 || got.Services[0] != valid.Services[0] {
		t.Fatalf("unexpected validated service or review ID: %+v", got)
	}
	check(forwarded.RequestID, http.StatusForbidden)

	approval := testMCPDetailMessage(t, protocol.TypeMCPTrust, "mobile-2", request.SessionID, protocol.MCPTrustRequest{Action: "approve", ServerName: "demo", ReviewID: valid.ReviewID})
	approval.ClientToken = token
	writeTestMessage(t, client, approval)
	forwardedApprove := readTestMessage(t, agent, protocol.TypeMCPTrustInternal)
	readTestMessage(t, client, protocol.TypeHeartbeat)
	var approveInternal protocol.MCPTrustInternal
	if err := json.Unmarshal(forwardedApprove.Payload, &approveInternal); err != nil || approveInternal.ConnectionID != internal.ConnectionID || forwardedApprove.RequestID == forwarded.RequestID {
		t.Fatalf("did not retain the socket identity with fresh correlation: %+v (%v)", approveInternal, err)
	}
	check(forwardedApprove.RequestID, http.StatusNoContent)
	check(forwardedApprove.RequestID, http.StatusForbidden)
	legacyReply := testMCPDetailMessage(t, protocol.TypeError, forwardedApprove.RequestID, approval.SessionID, map[string]string{"message": "must-not-leak secret", "request_type": string(protocol.TypeMCPTrustInternal)})
	writeTestMessage(t, agent, legacyReply)
	got = testMCPTrustResult(t, client, approval, "approve", "demo", protocol.MCPTrustUnavailable)
	if len(got.Services) != 0 || got.ReviewID != "" {
		t.Fatal("legacy error leaked result details")
	}
}

func TestRelayMCPTrustCheckRejectsInactivePairingAndSockets(t *testing.T) {
	for _, reason := range []string{"revoked in store", "expired route", "expired pairing", "mobile disconnect", "socket identity switch", "agent replaced", "agent offline"} {
		t.Run(reason, func(t *testing.T) {
			s, _, _, _, token := mcpTrustFixture(t)
			client := &peer{mcpTrustTransport: true}
			message := testMCPDetailMessage(t, protocol.TypeMCPTrust, "mobile-route", "session", nil)
			s.registerClientConnection(client, message.UserID, message.DeviceID, token, "local")
			id := clientTokenHash(token)
			route := s.registerMCPTrust(client, message, id, protocol.MCPTrustRequest{Action: "list"})
			if route == nil || !s.checkMCPTrustRoute(context.Background(), agentKey(message.UserID, message.DeviceID), route.mcpTrust.agentID) {
				t.Fatal("could not check active read route")
			}
			switch reason {
			case "revoked in store":
				if err := s.authStore.Revoke(context.Background(), id, time.Now()); err != nil {
					t.Fatal(err)
				}
			case "expired pairing":
				pairing, err := s.authStore.Get(context.Background(), id)
				if err != nil {
					t.Fatal(err)
				}
				pairing.ExpiresAt = time.Now().Add(-time.Second)
				if err := s.authStore.Save(context.Background(), pairing); err != nil {
					t.Fatal(err)
				}
			case "expired route":
				s.clientLock.Lock()
				route.ConnectedAt = time.Now().Add(-mcpTrustRouteTTL)
				s.clientLock.Unlock()
				s.touchClient(message.RequestID)
				s.touchClientPeer(client)
			case "mobile disconnect":
				s.unregisterClientPeer(client)
				s.registerClientConnection(&peer{mcpTrustTransport: true}, message.UserID, message.DeviceID, token, "replacement")
			case "socket identity switch":
				s.registerClientConnection(client, message.UserID, message.DeviceID, "different-token", "local")
				s.registerClientConnection(client, message.UserID, message.DeviceID, token, "local")
			case "agent replaced":
				s.registerAgent(&peer{mcpTrustTransport: true}, message.UserID, message.DeviceID, "another-code", "local")
			case "agent offline":
				s.unregisterAgent(route.mcpTrust.agent, message.UserID, message.DeviceID)
			}
			if s.checkMCPTrustRoute(context.Background(), agentKey(message.UserID, message.DeviceID), route.mcpTrust.agentID) {
				t.Fatal("invalidated route still authorizes Agent check")
			}
		})
	}
}

func TestRelayMCPTrustNoPlaintextOverride(t *testing.T) {
	s := newTestServer()
	r := httptest.NewRequest(http.MethodPost, "http://localhost"+protocol.MCPTrustCheckPath, nil)
	local := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 18080}
	r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, local))
	r.RemoteAddr = "127.0.0.1:10000"
	if !s.mcpTrustTransportAllowed(r) {
		t.Fatal("direct loopback development request denied")
	}
	s.mcpTrustLocalDev = false
	if s.mcpTrustTransportAllowed(r) {
		t.Fatal("unconfigured loopback accepted plaintext")
	}
	s.mcpTrustLocalDev = true
	r.Host = "public.example"
	if s.mcpTrustTransportAllowed(r) {
		t.Fatal("public Host was accepted over local plaintext")
	}
	r.Host = "localhost:18080"
	r.RemoteAddr = "192.0.2.1:10000"
	r.Header.Set("X-Forwarded-Proto", "https")
	if s.mcpTrustTransportAllowed(r) {
		t.Fatal("forwarded TLS was accepted over remote plaintext")
	}
	tlsRequest := httptest.NewRequest(http.MethodPost, "https://public.example"+protocol.MCPTrustCheckPath, nil)
	if !s.mcpTrustTransportAllowed(tlsRequest) {
		t.Fatal("TLS was not accepted")
	}
}

func TestRelayMCPTrustWithTLS(t *testing.T) {
	s := newTestServer()
	s.mcpTrustLocalDev = false
	server := httptest.NewTLSServer(s.routes())
	defer server.Close()
	base := "wss" + strings.TrimPrefix(server.URL, "https")
	dial := func(path string, headers http.Header) *websocket.Conn {
		t.Helper()
		dialer := websocket.Dialer{TLSClientConfig: server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()}
		dialer.TLSClientConfig.MinVersion = tls.VersionTLS12
		conn, _, err := dialer.Dial(base+path, headers)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		return conn
	}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer test-agent-token")
	headers.Set(protocol.HeaderAgentUserID, "local")
	headers.Set(protocol.HeaderAgentDeviceID, "pc-local")
	agent := dial("/ws/agent", headers)
	writeAgentOnline(t, agent, "local", "pc-local", "123456")
	readTestMessage(t, agent, protocol.TypeHeartbeat)
	client := dial("/ws/client", nil)
	request := testMCPDetailMessage(t, protocol.TypeMCPTrust, "mobile-tls", "", protocol.MCPTrustRequest{Action: "list"})
	request.ClientToken = pairTestClient(t, server, "123456")
	writeTestMessage(t, client, request)
	forwarded := readTestMessage(t, agent, protocol.TypeMCPTrustInternal)
	postMCPTrustCheck(t, server, http.MethodPost, forwarded.RequestID, "test-agent-token", "local", "pc-local", nil, http.StatusNoContent)
}

func TestRelayMCPTrustPendingCaps(t *testing.T) {
	s := newTestServer()
	s.registerAgent(&peer{mcpTrustTransport: true}, "local", "pc-local", "123456", "local")
	payload := protocol.MCPTrustRequest{Action: "list"}
	register := func(requestID, token string) *clientEntry {
		client := &peer{mcpTrustTransport: true}
		s.registerClientConnection(client, "local", "pc-local", token, "local")
		message := testMCPDetailMessage(t, protocol.TypeMCPTrust, requestID, "session", payload)
		return s.registerMCPTrust(client, message, clientTokenHash(token), payload)
	}
	for i := 0; i < maxMCPTrustPerPairing; i++ {
		if register(fmt.Sprintf("pairing-%d", i), "same-pairing") == nil {
			t.Fatal("route rejected before reaching per-pairing cap")
		}
	}
	if register("over-pairing-cap", "same-pairing") != nil {
		t.Fatal("per-pairing cap bypassed using another socket")
	}
	for i := maxMCPTrustPerPairing; i < maxMCPTrustRoutes; i++ {
		id := fmt.Sprintf("global-%d", i)
		if register(id, id) == nil {
			t.Fatal("route rejected before reaching global cap")
		}
	}
	if register("over-global-cap", "new-pairing") != nil {
		t.Fatal("global cap bypassed using another pairing")
	}
}

func TestRelayMCPTrustBoundsAndSuccessValidation(t *testing.T) {
	s, server, agent, client, token := mcpTrustFixture(t)
	request := testMCPDetailMessage(t, protocol.TypeMCPTrust, "mobile-cap", "session", protocol.MCPTrustRequest{Action: "list"})
	request.ClientToken = token
	oversized := request
	oversized.Payload = []byte(`{"action":"list","extra":"` + strings.Repeat("a", protocol.MCPTrustMaxRequestBytes) + `"}`)
	writeTestMessage(t, client, oversized)
	readTestMessage(t, client, protocol.TypeError)
	writeTestMessage(t, client, request)
	forwarded := readTestMessage(t, agent, protocol.TypeMCPTrustInternal)
	readTestMessage(t, client, protocol.TypeHeartbeat)
	if route := s.getClient(request.RequestID); route == nil || s.rebindClientRequest(request.RequestID, request.UserID, request.DeviceID, token, new(peer)) ||
		s.registerClient(request.RequestID, protocol.TypeMCPList, new(peer), request.UserID, request.DeviceID, token, "other") {
		t.Fatal("mcp trust route is missing or could be rebound or overwritten")
	}
	unsafe := testMCPTrustService()
	unsafe.Endpoint = "https://mcp.example/?secret=hidden"
	if validMCPTrustResult(protocol.MCPTrustRequest{Action: "list"}, protocol.MCPTrustResult{Services: []protocol.MCPTrustService{unsafe}}) {
		t.Fatal("unsafe result endpoint passed validation")
	}
	writeTestMessage(t, agent, testMCPDetailMessage(t, protocol.TypeMCPTrustResult, forwarded.RequestID, request.SessionID, protocol.MCPTrustResult{Action: "list", Services: []protocol.MCPTrustService{testMCPTrustService()}}))
	testMCPTrustResult(t, client, request, "list", "", protocol.MCPTrustUnavailable)

	request.RequestID = "mobile-cap-2"
	writeTestMessage(t, client, request)
	forwarded = readTestMessage(t, agent, protocol.TypeMCPTrustInternal)
	readTestMessage(t, client, protocol.TypeHeartbeat)
	postMCPTrustCheck(t, server, http.MethodPost, forwarded.RequestID, "test-agent-token", "local", "pc-local", nil, http.StatusNoContent)
	oversizedReply := testMCPDetailMessage(t, protocol.TypeMCPTrustResult, forwarded.RequestID, request.SessionID, map[string]any{
		"action": "list", "services": []protocol.MCPTrustService{testMCPTrustService()}, "unknown": strings.Repeat("x", protocol.MCPTrustMaxResponseBytes),
	})
	writeTestMessage(t, agent, oversizedReply)
	testMCPTrustResult(t, client, request, "list", "", protocol.MCPTrustUnavailable)
}
