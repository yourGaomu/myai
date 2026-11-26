package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domainauthorization "myai/core/domain/authorization"
	authorizationport "myai/core/port/authorization"
	"myai/core/remote/protocol"
)

type clientAuthTestStore struct {
	authorizationport.Store
	record   domainauthorization.ClientAuthorization
	getErr   error
	touchErr error
}

func (s clientAuthTestStore) Get(context.Context, string) (domainauthorization.ClientAuthorization, error) {
	return s.record, s.getErr
}

func (s clientAuthTestStore) Touch(context.Context, string, time.Time) error {
	return s.touchErr
}

func TestClientMessageAuthErrorsAreTypedAndFailClosed(t *testing.T) {
	const token = "test-client-token"
	now := time.Now()
	valid := domainauthorization.ClientAuthorization{ID: clientTokenHash(token), UserID: "local", DeviceID: "pc-local", ExpiresAt: now.Add(time.Hour)}
	expired, revoked := valid, valid
	expired.ExpiresAt = now.Add(-time.Hour)
	revoked.RevokedAt = &now
	notFound := fmt.Errorf("test lookup: %w", authorizationport.ErrNotFound)
	for _, tc := range []struct {
		name, deviceID, token string
		store                 authorizationport.Store
		wantCode              string
	}{
		{"missing token", valid.DeviceID, "", nil, "client_auth_rejected"},
		{"mismatched identity", "other-device", token, clientAuthTestStore{record: valid}, "client_auth_rejected"},
		{"empty record", valid.DeviceID, token, clientAuthTestStore{}, "client_auth_rejected"},
		{"not found at get", valid.DeviceID, token, clientAuthTestStore{getErr: notFound}, "client_auth_rejected"},
		{"not found at touch", valid.DeviceID, token, clientAuthTestStore{record: valid, touchErr: notFound}, "client_auth_rejected"},
		{"expired", valid.DeviceID, token, clientAuthTestStore{record: expired}, "client_auth_rejected"},
		{"revoked", valid.DeviceID, token, clientAuthTestStore{record: revoked}, "client_auth_rejected"},
		{"get unavailable", valid.DeviceID, token, clientAuthTestStore{getErr: context.DeadlineExceeded}, "client_auth_unavailable"},
		{"touch unavailable", valid.DeviceID, token, clientAuthTestStore{record: valid, touchErr: context.Canceled}, "client_auth_unavailable"},
		{"no store", valid.DeviceID, token, nil, "client_auth_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer()
			s.authStore = tc.store
			httpServer := httptest.NewServer(s.routes())
			defer httpServer.Close()
			client := dialTestWebSocket(t, "ws"+strings.TrimPrefix(httpServer.URL, "http")+"/ws/client")
			defer client.Close()
			for _, kind := range []protocol.MessageType{protocol.TypeHeartbeat, protocol.TypeUserMessage} {
				request := protocol.Message{Type: kind, RequestID: "request-" + string(kind), UserID: valid.UserID, DeviceID: tc.deviceID, ClientToken: tc.token}
				writeTestMessage(t, client, request)
				response := readTestMessage(t, client, protocol.TypeError)
				payload, err := protocol.DecodePayload[map[string]string](response)
				if err != nil || response.RequestID != request.RequestID || payload["message"] != "client token is invalid or expired" || payload["request_type"] != string(kind) || payload["error_code"] != tc.wantCode {
					t.Fatalf("unexpected auth response: request=%q payload=%s err=%v", response.RequestID, response.Payload, err)
				}
				if s.getClient(request.RequestID) != nil || len(s.getClientConnections(request.UserID, request.DeviceID)) != 0 {
					t.Fatal("failed authentication registered a route or connection")
				}
			}
		})
	}
}

func TestClientHeartbeatAckAndUnrelatedErrorPreserveWireShape(t *testing.T) {
	s := newTestServer()
	const userID, deviceID = "local", "pc-local"
	token, auth, err := s.authorizeClient(userID, deviceID, "test", "")
	if err != nil {
		t.Fatal(err)
	}
	auth.LastSeenAt = time.Time{}
	if err := s.authStore.Save(context.Background(), auth); err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(s.routes())
	defer httpServer.Close()
	client := dialTestWebSocket(t, "ws"+strings.TrimPrefix(httpServer.URL, "http")+"/ws/client")
	defer client.Close()
	for _, request := range []protocol.Message{
		{Type: protocol.TypeHeartbeat, RequestID: "legacy"},
		{Type: protocol.TypeHeartbeat, RequestID: "authenticated", UserID: userID, DeviceID: deviceID, ClientToken: token},
	} {
		writeTestMessage(t, client, request)
		response := readTestMessage(t, client, protocol.TypeHeartbeat)
		payload, err := protocol.DecodePayload[map[string]string](response)
		if err != nil || response.RequestID != request.RequestID || len(payload) != 2 || payload["role"] != "client" || payload["received"] != "heartbeat" {
			t.Fatalf("unexpected heartbeat ACK: request=%q payload=%s err=%v", response.RequestID, response.Payload, err)
		}
	}
	updated, err := s.authStore.Get(context.Background(), auth.ID)
	if err != nil || updated.LastSeenAt.IsZero() {
		t.Fatalf("authenticated heartbeat did not update last-seen time: %v", err)
	}

	writeTestMessage(t, client, protocol.Message{Type: protocol.TypePermissionResult, RequestID: "no-grant", UserID: userID, DeviceID: deviceID, ClientToken: token, Payload: json.RawMessage(`{"approval_id":"unknown"}`)})
	response := readTestMessage(t, client, protocol.TypeError)
	payload, err := protocol.DecodePayload[map[string]string](response)
	if err != nil || response.RequestID != "no-grant" || payload["request_type"] != "permission_result" || payload["approval_id"] != "unknown" {
		t.Fatalf("unexpected permission error: request=%q payload=%s err=%v", response.RequestID, response.Payload, err)
	}
	if _, present := payload["error_code"]; present {
		t.Fatal("non-auth error carried an auth error code")
	}
}

func TestIntentTraceTraversesRelayAsPartOfChatRequest(t *testing.T) {
	s := newTestServer()
	server := httptest.NewServer(s.routes())
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	agent := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agent.Close()
	writeAgentOnline(t, agent, "local", "pc-local", "123456")
	readTestMessage(t, agent, protocol.TypeHeartbeat)
	token := pairTestClient(t, server, "123456")
	client := dialTestWebSocket(t, wsURL+"/ws/client")
	defer client.Close()
	writeTestMessage(t, client, protocol.Message{Type: protocol.TypeUserMessage, RequestID: "turn", UserID: "local", DeviceID: "pc-local", SessionID: "session", ClientToken: token})
	readTestMessage(t, agent, protocol.TypeUserMessage)
	readTestMessage(t, client, protocol.TypeHeartbeat)
	for _, status := range []string{"started", "succeeded"} {
		payload := protocol.IntentTracePayload{TraceID: "trace", SessionID: "session", RequestID: "turn", Status: status, RequestBody: `{ "input": "demo" }`}
		if status == "succeeded" {
			payload.ResponseBody = `{ "intent": "conversation" }`
		}
		message, err := protocol.NewMessage(protocol.TypeIntentTraceEvent, "turn", "local", "pc-local", "session", payload)
		if err != nil {
			t.Fatal(err)
		}
		writeTestMessage(t, agent, message)
		response := readTestMessage(t, client, protocol.TypeIntentTraceEvent)
		got, err := protocol.DecodePayload[protocol.IntentTracePayload](response)
		if err != nil || response.RequestID != "turn" || got.TraceID != "trace" || got.Status != status || got.RequestBody != payload.RequestBody || got.ResponseBody != payload.ResponseBody {
			t.Fatalf("unexpected intent event: %#v %v", response, err)
		}
	}
	writeTestMessage(t, agent, protocol.Message{Type: protocol.TypeAssistantDone, RequestID: "turn", UserID: "local", DeviceID: "pc-local", SessionID: "session"})
	readTestMessage(t, client, protocol.TypeAssistantDone)
}

func TestQueuedMessageTraversesRelayAndFinishesRoute(t *testing.T) {
	s := newTestServer()
	httpServer := httptest.NewServer(s.routes())
	defer httpServer.Close()
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	agent := dialTestWebSocket(t, wsURL+"/ws/agent")
	defer agent.Close()
	writeAgentOnline(t, agent, "local", "pc-local", "123456")
	readTestMessage(t, agent, protocol.TypeHeartbeat)
	token := pairTestClient(t, httpServer, "123456")
	client := dialTestWebSocket(t, wsURL+"/ws/client")
	defer client.Close()
	writeTestMessage(t, client, protocol.Message{Type: protocol.TypeUserMessage, RequestID: "queued", UserID: "local", DeviceID: "pc-local", ClientToken: token})
	readTestMessage(t, agent, protocol.TypeUserMessage)
	readTestMessage(t, client, protocol.TypeHeartbeat)
	writeTestMessage(t, agent, protocol.Message{Type: protocol.TypeUserMessageQueued, RequestID: "queued", UserID: "local", DeviceID: "pc-local"})
	readTestMessage(t, client, protocol.TypeUserMessageQueued)
	// A heartbeat on the same agent stream is processed after route cleanup.
	writeTestMessage(t, agent, protocol.Message{Type: protocol.TypeHeartbeat, UserID: "local", DeviceID: "pc-local"})
	readTestMessage(t, agent, protocol.TypeHeartbeat)
	if s.getClient("queued") != nil {
		t.Fatal("queue acknowledgement retained route")
	}
}

func TestRevocationRetiresConnectionsAndRoutes(t *testing.T) {
	s := newTestServer()
	token, auth, err := s.authorizeClient("user", "device", "browser", "")
	if err != nil {
		t.Fatal(err)
	}
	p := &peer{}
	s.registerClientConnection(p, "user", "device", token, "")
	s.registerClient("turn", protocol.TypeUserMessage, p, "user", "device", token, "")
	if err := s.revokeClientAuthorization(auth.ID, "user", "device"); err != nil {
		t.Fatal(err)
	}
	if len(s.getClientConnections("user", "device")) != 0 || s.getClient("turn") != nil {
		t.Fatal("revoked client retained access")
	}
}

func TestDeliveryRejectsExpiredOrExternallyRevokedAuthorization(t *testing.T) {
	for _, revoked := range []bool{false, true} {
		s := newTestServer()
		token, auth, err := s.authorizeClient("user", "device", "browser", "")
		if err != nil {
			t.Fatal(err)
		}
		p := &peer{}
		s.registerClientConnection(p, "user", "device", token, "")
		s.registerClient("turn", protocol.TypeUserMessage, p, "user", "device", token, "")
		if revoked {
			err = s.authStore.Revoke(context.Background(), auth.ID, time.Now())
		} else {
			auth.ExpiresAt = time.Now().Add(-time.Second)
			err = s.authStore.Save(context.Background(), auth)
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := s.writeAuthorizedClient(p, protocol.Message{Type: protocol.TypeAssistantDelta}); err == nil {
			t.Fatal("invalid authorization passed delivery check")
		}
		if len(s.getClientConnections("user", "device")) != 0 || s.getClient("turn") != nil {
			t.Fatal("invalid client retained routes")
		}
	}
}
