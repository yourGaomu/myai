package relay

import (
	"myai/core/remote/protocol"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRelayInspectionRoutesReleaseAfterReply(t *testing.T) {
	for _, pair := range [][2]protocol.MessageType{
		{protocol.TypeSubagentTaskDetail, protocol.TypeSubagentTaskDetailResult},
		{protocol.TypeSubagentTaskEvents, protocol.TypeSubagentTaskEventsResult},
	} {
		t.Run(string(pair[0]), func(t *testing.T) {
			server := newTestServer()
			testServer := httptest.NewServer(server.routes())
			defer testServer.Close()
			wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")
			agentConn := dialTestWebSocket(t, wsURL+"/ws/agent")
			defer agentConn.Close()
			clientConn := dialTestWebSocket(t, wsURL+"/ws/client")
			defer clientConn.Close()
			writeAgentOnline(t, agentConn, "local", "pc-local", "123456")
			readTestMessage(t, agentConn, protocol.TypeHeartbeat)
			token := pairTestClient(t, testServer, "123456")
			writeTestMessage(t, clientConn, protocol.Message{Type: pair[0], RequestID: "inspect", UserID: "local", DeviceID: "pc-local", ClientToken: token, SessionID: "parent"})
			readTestMessage(t, agentConn, pair[0])
			readTestMessage(t, clientConn, protocol.TypeHeartbeat)
			writeTestMessage(t, agentConn, protocol.Message{Type: pair[1], RequestID: "inspect", UserID: "local", DeviceID: "pc-local", SessionID: "parent"})
			readTestMessage(t, clientConn, pair[1])
			readTestMessage(t, agentConn, protocol.TypeHeartbeat)
			if server.getClient("inspect") != nil {
				t.Fatal("inspection leaked a request route")
			}
		})
	}
}
