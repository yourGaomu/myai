package relay

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

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
