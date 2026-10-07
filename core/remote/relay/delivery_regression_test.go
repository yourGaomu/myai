package relay

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"myai/core/remote/protocol"
)

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
