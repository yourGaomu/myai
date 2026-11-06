package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	domaininteraction "myai/core/domain/interaction"
	"myai/core/llm"
	"myai/core/remote/protocol"
	"myai/core/service"
)

type backgroundControlFacade struct {
	ChatFacade
	mu                sync.Mutex
	permissionPending bool
	questionPending   bool
}

func (f *backgroundControlFacade) ResolveBackgroundPermission(_ context.Context, sessionID, turnID, approvalID string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.permissionPending || sessionID != "session" || turnID != "turn" || approvalID != "approval" {
		return errors.New("no matching approval")
	}
	f.permissionPending = false
	return nil
}
func (f *backgroundControlFacade) ResolveBackgroundQuestion(_ context.Context, sessionID, turnID, questionID string, answer domaininteraction.Answer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.questionPending || sessionID != "session" || turnID != "turn" || questionID != "question" || answer.Answers["choice"] != "yes" {
		return errors.New("invalid question answer")
	}
	f.questionPending = false
	return nil
}
func (f *backgroundControlFacade) ListBackgroundQuestions(sessionID string) []domaininteraction.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.questionPending || sessionID != "" && sessionID != "session" {
		return nil
	}
	return []domaininteraction.Request{{ID: "question", SessionID: "session", RequestID: "turn", Questions: []domaininteraction.Question{{ID: "choice", Prompt: "Proceed?", Options: []string{"yes", "no"}}}}}
}

func TestBackgroundReplyFacadeValidatesIdentityAndOnlyFirstAnswerWins(t *testing.T) {
	for _, kind := range []string{"permission", "question"} {
		t.Run(kind, func(t *testing.T) {
			facade := &backgroundControlFacade{permissionPending: true, questionPending: true}
			a := &Agent{chatService: facade, permissionWaiters: newPermissionWaiterRegistry(), userQuestionWaiters: newUserQuestionWaiterRegistry()}
			messageType, payload := protocol.TypePermissionResult, any(protocol.PermissionResultPayload{ApprovalID: "approval", Allowed: true})
			if kind == "question" {
				messageType, payload = protocol.TypeUserQuestionAnswer, protocol.UserQuestionAnswerPayload{QuestionID: "question", Answers: map[string]string{"choice": "yes"}}
			}
			message, err := protocol.NewMessage(messageType, "turn", "user", "device", "session", payload)
			if err != nil {
				t.Fatal(err)
			}
			resolve := func(message protocol.Message) error {
				if kind == "permission" {
					return a.handlePermissionResult(message)
				}
				return a.handleUserQuestionAnswer(message)
			}
			wrong := message
			wrong.SessionID = "wrong"
			if resolve(wrong) == nil {
				t.Fatal("wrong session was accepted")
			}
			wrong = message
			wrong.RequestID = "wrong"
			if resolve(wrong) == nil {
				t.Fatal("wrong turn was accepted")
			}
			var wins atomic.Int32
			var wg sync.WaitGroup
			for i := 0; i < 16; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if resolve(message) == nil {
						wins.Add(1)
					}
				}()
			}
			wg.Wait()
			if wins.Load() != 1 {
				t.Fatalf("accepted %d replies, want 1", wins.Load())
			}
		})
	}
}

func TestBackgroundPayloadUsesPublicDTOsAndPreservesApproval(t *testing.T) {
	event := service.BackgroundTurnEvent{TurnID: "turn", SessionID: "session", ApprovalID: "approval", Permission: &llm.ToolPermissionRequest{Name: "write", Arguments: `{"path":"file"}`}, Question: &domaininteraction.Request{ID: "question", Questions: []domaininteraction.Question{{ID: "choice", Prompt: "Proceed?"}}}}
	payload := backgroundTurnPayload(event)
	if payload.Permission == nil || payload.Permission.ApprovalID != "approval" || payload.Permission.Name != "write" {
		t.Fatal("permission identity lost")
	}
	if payload.Question == nil || payload.Question.QuestionID != "question" || payload.TurnID != "turn" {
		t.Fatal("question identity lost")
	}
}

func TestBackgroundQuestionListDoesNotResolveStillPendingQuestion(t *testing.T) {
	for _, sessionFilter := range []string{"", "another-session"} {
		t.Run("filter="+sessionFilter, func(t *testing.T) {
			facade := &backgroundControlFacade{questionPending: true}
			a := &Agent{chatService: facade, userQuestionWaiters: newUserQuestionWaiterRegistry()}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				var message protocol.Message
				if conn.ReadJSON(&message) == nil {
					_ = a.handleUserQuestionList(conn, message)
				}
			}))
			defer server.Close()
			conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			message, _ := protocol.NewMessage(protocol.TypeUserQuestionList, "list", "user", "device", "", protocol.UserQuestionListPayload{SessionID: sessionFilter, KnownQuestionIDs: []string{"question", "finished"}})
			if err := conn.WriteJSON(message); err != nil {
				t.Fatal(err)
			}
			_ = conn.SetReadDeadline(time.Now().Add(time.Second))
			var result protocol.Message
			if err := conn.ReadJSON(&result); err != nil {
				t.Fatal(err)
			}
			payload, err := protocol.DecodePayload[protocol.UserQuestionListResultPayload](result)
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range payload.ResolvedQuestionIDs {
				if id == "question" {
					t.Fatal("pending background question marked resolved")
				}
			}
			if len(payload.ResolvedQuestionIDs) != 1 || payload.ResolvedQuestionIDs[0] != "finished" {
				t.Fatalf("wrong resolved IDs %#v", payload.ResolvedQuestionIDs)
			}
			if sessionFilter == "" && (len(payload.Questions) != 1 || payload.Questions[0].RequestID != "turn") {
				t.Fatal("pending question lost turn identity")
			}
		})
	}
}
