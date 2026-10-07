package relay

import (
	"net/http/httptest"
	"strings"
	"testing"

	"myai/core/remote/protocol"
)

func TestBackgroundTurnDeliveredAfterForegroundRouteRetired(t *testing.T) {
	s := newTestServer()
	httpServer := httptest.NewServer(s.routes())
	defer httpServer.Close()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	agent := dialTestWebSocket(t, url+"/ws/agent")
	defer agent.Close()
	writeAgentOnline(t, agent, "local", "pc-local", "123456")
	readTestMessage(t, agent, protocol.TypeHeartbeat)
	token := pairTestClient(t, httpServer, "123456")
	client := dialTestWebSocket(t, url+"/ws/client")
	defer client.Close()
	writeTestMessage(t, client, protocol.Message{Type: protocol.TypeUserMessage, RequestID: "foreground", UserID: "local", DeviceID: "pc-local", ClientToken: token, SessionID: "parent"})
	readTestMessage(t, agent, protocol.TypeUserMessage)
	readTestMessage(t, client, protocol.TypeHeartbeat)
	writeTestMessage(t, agent, protocol.Message{Type: protocol.TypeAssistantDone, RequestID: "foreground", UserID: "local", DeviceID: "pc-local", SessionID: "parent"})
	readTestMessage(t, client, protocol.TypeAssistantDone)
	// Send the unsolicited event on the same stream, after foreground cleanup.
	event, err := protocol.NewMessage(protocol.TypeBackgroundTurnEvent, "new-turn", "local", "pc-local", "parent", protocol.BackgroundTurnEventPayload{TurnID: "new-turn", SessionID: "parent", RunID: "new-run", Sequence: 1, Kind: "started"})
	if err != nil {
		t.Fatal(err)
	}
	writeTestMessage(t, agent, event)
	got := readTestMessage(t, client, protocol.TypeBackgroundTurnEvent)
	if got.RequestID != "new-turn" || got.SessionID != "parent" {
		t.Fatalf("wrong background identity: %#v", got)
	}
	if s.getClient("foreground") != nil || s.getClient("new-turn") != nil {
		t.Fatal("background event created or retained a request route")
	}
}
