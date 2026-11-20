package relay

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"myai/core/remote/protocol"
)

func TestControlErrorAndConfirmationDoNotRetireChatRoute(t *testing.T) {
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
	writeTestMessage(t, client, protocol.Message{Type: protocol.TypeUserMessage, RequestID: "turn", UserID: "local", DeviceID: "pc-local", ClientToken: token, SessionID: "session"})
	readTestMessage(t, agent, protocol.TypeUserMessage)
	readTestMessage(t, client, protocol.TypeHeartbeat)
	for _, tc := range []struct {
		kind    protocol.MessageType
		payload string
	}{
		{protocol.TypeError, `{"message":"missing approval ID","request_type":"permission_result"}`},
		{protocol.TypeError, `{"message":"invalid question","request_type":"user_question_answer","question_id":"question"}`},
		{protocol.TypePermissionResult, `{"approval_id":"approval","allowed":true}`},
	} {
		writeTestMessage(t, agent, protocol.Message{Type: tc.kind, RequestID: "turn", UserID: "local", DeviceID: "pc-local", SessionID: "session", Payload: json.RawMessage(tc.payload)})
		readTestMessage(t, client, tc.kind)
		readTestMessage(t, agent, protocol.TypeHeartbeat)
		if s.getClient("turn") == nil {
			t.Fatal("control response retired chat route")
		}
	}
	writeTestMessage(t, agent, protocol.Message{Type: protocol.TypeAssistantDone, RequestID: "turn", UserID: "local", DeviceID: "pc-local", SessionID: "session"})
	readTestMessage(t, client, protocol.TypeAssistantDone)
	readTestMessage(t, agent, protocol.TypeHeartbeat)
	if s.getClient("turn") != nil {
		t.Fatal("assistant done did not retire chat route")
	}
}

func TestForegroundPermissionReplaysOnlyForOriginalPairing(t *testing.T) {
	s := newTestServer()
	httpServer := httptest.NewServer(s.routes())
	defer httpServer.Close()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	agent := dialTestWebSocket(t, url+"/ws/agent")
	defer agent.Close()
	writeAgentOnline(t, agent, "local", "pc-local", "123456")
	readTestMessage(t, agent, protocol.TypeHeartbeat)
	token := pairTestClient(t, httpServer, "123456")
	writeAgentOnline(t, agent, "local", "pc-local", "654321")
	readTestMessage(t, agent, protocol.TypeHeartbeat)
	otherToken := pairTestClient(t, httpServer, "654321")

	turn := protocol.Message{Type: protocol.TypeUserMessage, RequestID: "turn", UserID: "local", DeviceID: "pc-local", SessionID: "session", ClientToken: token}
	ask := protocol.Message{Type: protocol.TypePermissionAsk, RequestID: turn.RequestID, UserID: turn.UserID, DeviceID: turn.DeviceID, SessionID: turn.SessionID,
		Payload: json.RawMessage(`{"approval_id":"approval","name":"write_file","require_fresh_consent":true}`)}
	owner := dialTestWebSocket(t, url+"/ws/client")
	defer owner.Close()
	writeTestMessage(t, owner, turn)
	readTestMessage(t, agent, protocol.TypeUserMessage)
	readTestMessage(t, owner, protocol.TypeHeartbeat)

	heartbeat := protocol.Message{Type: protocol.TypeHeartbeat, RequestID: "heartbeat", UserID: turn.UserID, DeviceID: turn.DeviceID, ClientToken: otherToken}
	other := dialTestWebSocket(t, url+"/ws/client")
	defer other.Close()
	writeTestMessage(t, other, heartbeat)
	readTestMessage(t, other, protocol.TypeHeartbeat)
	writeTestMessage(t, agent, ask)
	original := readTestMessage(t, owner, protocol.TypePermissionAsk)
	readTestMessage(t, agent, protocol.TypeHeartbeat)
	writeTestMessage(t, other, heartbeat)
	readTestMessage(t, other, protocol.TypeHeartbeat) // No live broadcast.

	otherReconnect := dialTestWebSocket(t, url+"/ws/client")
	defer otherReconnect.Close()
	writeTestMessage(t, otherReconnect, heartbeat)
	readTestMessage(t, otherReconnect, protocol.TypeHeartbeat) // No cross-pairing replay.
	answer := ask
	answer.Type, answer.ClientToken = protocol.TypePermissionResult, otherToken
	answer.Payload = json.RawMessage(`{"approval_id":"approval","allowed":true}`)
	writeTestMessage(t, otherReconnect, answer)
	readTestMessage(t, otherReconnect, protocol.TypeError)

	ownerReconnect := dialTestWebSocket(t, url+"/ws/client")
	defer ownerReconnect.Close()
	heartbeat.ClientToken = token
	writeTestMessage(t, ownerReconnect, heartbeat)
	if replayed := readTestMessage(t, ownerReconnect, protocol.TypePermissionAsk); !reflect.DeepEqual(replayed, original) {
		t.Fatalf("reconnect changed the original permission frame: %#v", replayed)
	}
	readTestMessage(t, ownerReconnect, protocol.TypeHeartbeat)
	writeTestMessage(t, ownerReconnect, heartbeat)
	readTestMessage(t, ownerReconnect, protocol.TypeHeartbeat) // Replay is once per connection.
	// A reply racing the reconnect must go to the new socket even while the old
	// one is still open.
	confirmation := answer
	confirmation.ClientToken = ""
	writeTestMessage(t, agent, confirmation)
	readTestMessage(t, ownerReconnect, protocol.TypePermissionResult)
	readTestMessage(t, agent, protocol.TypeHeartbeat)
}

func TestForegroundPermissionReplayTracksTerminalResponsesWhileClientOffline(t *testing.T) {
	for _, tc := range []struct {
		name      string
		request   protocol.MessageType
		response  protocol.MessageType
		payload   string
		wantAsk   bool
		wantRoute bool
	}{
		{"matching confirmation", protocol.TypeUserMessage, protocol.TypePermissionResult, `{"approval_id":"approval","allowed":false}`, false, true},
		{"unrelated confirmation", protocol.TypeUserMessage, protocol.TypePermissionResult, `{"approval_id":"other","allowed":true}`, true, true},
		{"nonterminal control error", protocol.TypeUserMessage, protocol.TypeError, `{"request_type":"permission_result","approval_id":"approval"}`, true, true},
		{"assistant done", protocol.TypeUserMessage, protocol.TypeAssistantDone, "", false, false},
		{"terminal turn error", protocol.TypeUserMessage, protocol.TypeError, `{"request_type":"user_message","message":"failed"}`, false, false},
		{"plan assistant done before final result", protocol.TypeSessionPlanExecute, protocol.TypeAssistantDone, "", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			writeTestMessage(t, client, protocol.Message{Type: tc.request, RequestID: "turn", UserID: "local", DeviceID: "pc-local", SessionID: "session", ClientToken: token})
			readTestMessage(t, agent, tc.request)
			readTestMessage(t, client, protocol.TypeHeartbeat)
			ask := protocol.Message{Type: protocol.TypePermissionAsk, RequestID: "turn", UserID: "local", DeviceID: "pc-local", SessionID: "session", Payload: json.RawMessage(`{"approval_id":"approval"}`)}
			writeTestMessage(t, agent, ask)
			readTestMessage(t, client, protocol.TypePermissionAsk)
			readTestMessage(t, agent, protocol.TypeHeartbeat)
			client.Close()
			waitForClientDisconnect(t, s, "local", "pc-local")

			response := ask
			response.Type, response.Payload = tc.response, json.RawMessage(tc.payload)
			writeTestMessage(t, agent, response)
			readTestMessage(t, agent, protocol.TypeError) // No live client; cleanup still applies.
			if (s.getClient("turn") != nil) != tc.wantRoute {
				t.Fatalf("unexpected request route after %s", tc.response)
			}
			reconnected := dialTestWebSocket(t, url+"/ws/client")
			defer reconnected.Close()
			writeTestMessage(t, reconnected, protocol.Message{Type: protocol.TypeHeartbeat, UserID: "local", DeviceID: "pc-local", ClientToken: token})
			if tc.wantAsk {
				readTestMessage(t, reconnected, protocol.TypePermissionAsk)
			}
			readTestMessage(t, reconnected, protocol.TypeHeartbeat)
		})
	}
}

func waitForClientDisconnect(t *testing.T, s *Server, userID, deviceID string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for len(s.getClientConnections(userID, deviceID)) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("client connection was not unregistered")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestBackgroundInteractionRequiresGrantAndReplaysOnReconnect(t *testing.T) {
	for _, kind := range []string{"permission", "question"} {
		t.Run(kind, func(t *testing.T) {
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
			heartbeat := protocol.Message{Type: protocol.TypeHeartbeat, UserID: "local", DeviceID: "pc-local", ClientToken: token}
			writeTestMessage(t, client, heartbeat)
			readTestMessage(t, client, protocol.TypeHeartbeat)
			answerType, answerJSON := protocol.TypePermissionResult, `{"approval_id":"approval","allowed":true}`
			event := protocol.BackgroundTurnEventPayload{TurnID: "background", SessionID: "session", Sequence: 1, Kind: "permission_ask", ApprovalID: "approval", Permission: &protocol.PermissionAskPayload{ApprovalID: "approval", Name: "write"}}
			resolved := protocol.BackgroundTurnEventPayload{TurnID: "background", SessionID: "session", Sequence: 2, Kind: "permission_resolved", ApprovalID: "approval", Status: "allowed"}
			if kind == "question" {
				answerType, answerJSON = protocol.TypeUserQuestionAnswer, `{"question_id":"question","answers":{"choice":"yes"}}`
				event.Kind, event.ApprovalID, event.Permission = "user_question", "", nil
				event.Question = &protocol.UserQuestionPayload{QuestionID: "question", Questions: []protocol.UserQuestionItemPayload{{ID: "choice", Prompt: "Proceed?", Options: []string{"yes", "no"}}}}
				resolved.Kind, resolved.Question, resolved.ApprovalID = "user_question_resolved", event.Question, ""
			}
			answer := protocol.Message{Type: answerType, RequestID: "background", SessionID: "session", UserID: "local", DeviceID: "pc-local", ClientToken: token, Payload: json.RawMessage(answerJSON)}
			writeTestMessage(t, client, answer)
			rejected := readTestMessage(t, client, protocol.TypeError)
			if controlReplyID(rejected, "request_type") != string(answerType) {
				t.Fatal("control rejection missing type")
			}
			message, err := protocol.NewMessage(protocol.TypeBackgroundTurnEvent, "background", "local", "pc-local", "session", event)
			if err != nil {
				t.Fatal(err)
			}
			writeTestMessage(t, agent, message)
			readTestMessage(t, client, protocol.TypeBackgroundTurnEvent)
			readTestMessage(t, agent, protocol.TypeHeartbeat)
			wrong := answer
			wrong.SessionID = "another-session"
			writeTestMessage(t, client, wrong)
			readTestMessage(t, client, protocol.TypeError)
			_ = client.Close()
			reconnected := dialTestWebSocket(t, url+"/ws/client")
			defer reconnected.Close()
			writeTestMessage(t, reconnected, heartbeat)
			replayed := readTestMessage(t, reconnected, protocol.TypeBackgroundTurnEvent)
			if replayed.RequestID != "background" {
				t.Fatal("pending interaction lost turn identity")
			}
			readTestMessage(t, reconnected, protocol.TypeHeartbeat)
			writeTestMessage(t, reconnected, answer)
			readTestMessage(t, agent, answerType)
			readTestMessage(t, reconnected, protocol.TypeHeartbeat)
			if s.getClient("background") != nil {
				t.Fatal("background reply created foreground route")
			}
			resolution, err := protocol.NewMessage(protocol.TypeBackgroundTurnEvent, "background", "local", "pc-local", "session", resolved)
			if err != nil {
				t.Fatal(err)
			}
			writeTestMessage(t, agent, resolution)
			readTestMessage(t, reconnected, protocol.TypeBackgroundTurnEvent)
			readTestMessage(t, agent, protocol.TypeHeartbeat)
			writeTestMessage(t, reconnected, answer)
			readTestMessage(t, reconnected, protocol.TypeError)
		})
	}
}

func TestBackgroundResyncRetiresStaleApprovalCache(t *testing.T) {
	s := newTestServer()
	pending, _ := protocol.NewMessage(protocol.TypeBackgroundTurnEvent, "turn", "local", "pc-local", "session", protocol.BackgroundTurnEventPayload{TurnID: "turn", SessionID: "session", Kind: "permission_ask", ApprovalID: "approval"})
	if err := s.rememberInteraction(pending); err != nil {
		t.Fatal(err)
	}
	resync, _ := protocol.NewMessage(protocol.TypeBackgroundTurnEvent, "resync", "local", "pc-local", "", protocol.BackgroundTurnEventPayload{Kind: "resync_required"})
	if err := s.rememberInteraction(resync); err != nil {
		t.Fatal(err)
	}
	if len(s.interactions) != 0 {
		t.Fatal("stale approval survived Agent resync")
	}
	if err := s.rememberInteraction(pending); err != nil {
		t.Fatal(err)
	}
	if len(s.interactions) != 1 {
		t.Fatal("still-pending approval could not be replayed after resync")
	}
}

func TestBackgroundGrantBelongsToPairedClientAndRevocationRemovesIt(t *testing.T) {
	s := newTestServer()
	token, _, err := s.authorizeClient("local", "pc-local", "test", "test")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := s.authorizeClient("local", "pc-local", "other", "test")
	if err != nil {
		t.Fatal(err)
	}
	p, otherPeer := &peer{}, &peer{}
	s.registerClientConnection(p, "local", "pc-local", token, "test")
	message, _ := protocol.NewMessage(protocol.TypeBackgroundTurnEvent, "turn", "local", "pc-local", "session", protocol.BackgroundTurnEventPayload{TurnID: "turn", SessionID: "session", Kind: "permission_ask", ApprovalID: "approval"})
	if err := s.rememberInteraction(message); err != nil {
		t.Fatal(err)
	}
	s.grantInteraction(p, message)
	reply := protocol.Message{RequestID: "turn", UserID: "local", DeviceID: "pc-local", SessionID: "session", ClientToken: token}
	if !s.authorizeInteraction(p, reply, "permission", "approval") {
		t.Fatal("valid grant rejected")
	}
	s.clientLock.Lock()
	s.connections[otherPeer] = &clientConnection{UserID: "local", DeviceID: "pc-local", ClientID: clientTokenHash(other)}
	s.clientLock.Unlock()
	reply.ClientToken = other
	if s.authorizeInteraction(otherPeer, reply, "permission", "approval") {
		t.Fatal("another token inherited grant without delivery")
	}
	s.invalidateClientAuthorization(clientTokenHash(token))
	reply.ClientToken = token
	if s.authorizeInteraction(p, reply, "permission", "approval") {
		t.Fatal("revoked client retained grant")
	}
}
