package service

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	generationcommand "myai/core/application/chat/generation/command"
	generationresult "myai/core/application/chat/generation/result"
	domainagentrun "myai/core/domain/agentrun"
	domaininteraction "myai/core/domain/interaction"
	"myai/core/llm"
)

type backgroundTestQuestionResult struct {
	answer domaininteraction.Answer
	err    error
}

func backgroundTestReceive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case value, open := <-ch:
		if !open {
			t.Fatal("background result channel closed unexpectedly")
		}
		return value
	case <-timer.C:
		t.Fatal("background operation did not finish")
		var zero T
		return zero
	}
}

func backgroundTestPending(t *testing.T, s *ChatService, want int) {
	t.Helper()
	s.backgroundEventsMu.Lock()
	got := len(s.backgroundPending)
	s.backgroundEventsMu.Unlock()
	if got != want {
		t.Fatalf("pending background interactions = %d, want %d", got, want)
	}
}

func backgroundTestRequest(id string) domaininteraction.Request {
	return domaininteraction.Request{
		ID: id, SourceSessionID: "source-session", SourceTaskID: "source-task",
		RequestID: "model-request", RunID: "question-run", ToolCallID: "question-tool",
		Title: "Choose the next action",
		Questions: []domaininteraction.Question{
			{ID: "color", Prompt: "Which color?", Options: []string{"blue", "green"}},
			{ID: "details", Prompt: "Any details?", AllowOther: true},
		},
	}
}

func backgroundTestAnswer() domaininteraction.Answer {
	return domaininteraction.Answer{Answers: map[string]string{"color": "blue", "details": "Keep the current layout"}}
}

func backgroundTestAskQuestion(stream llm.ChatStreamHandler, ctx context.Context, request domaininteraction.Request) <-chan backgroundTestQuestionResult {
	result := make(chan backgroundTestQuestionResult, 1)
	go func() {
		answer, err := stream.OnUserQuestion(ctx, request)
		result <- backgroundTestQuestionResult{answer: answer, err: err}
	}()
	return result
}

func TestBackgroundPermissionRoundTrip(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		name, status := "allow", "allowed"
		if !allowed {
			name, status = "deny", "denied"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := NewChatService(ChatDependencies{})
				events, unsubscribe := s.SubscribeBackgroundTurns(8)
				defer unsubscribe()
				stream, finish := s.backgroundStreamWithIdentity("session", "turn", "wire-request")
				defer finish(ChatResponse{}, nil)
				request := llm.ToolPermissionRequest{ToolCallID: "tool-call", Name: "write_file", Arguments: `{"path":"notes.txt"}`, Mode: "ask"}
				result := make(chan bool, 1)
				go func() { result <- stream.OnToolAsk(request) }()
				ask := backgroundTestReceive(t, events)
				if ask.Kind != "permission_ask" || ask.ApprovalID == "" || ask.Sequence == 0 ||
					ask.SessionID != "session" || ask.TurnID != stream.CorrelationID || ask.RequestID != "wire-request" ||
					ask.Permission == nil || !reflect.DeepEqual(*ask.Permission, request) {
					t.Fatalf("invalid permission ask: %#v", ask)
				}

				for _, identity := range []struct{ session, turn, approval string }{
					{"other-session", ask.TurnID, ask.ApprovalID},
					{ask.SessionID, "other-turn", ask.ApprovalID},
					{ask.SessionID, ask.TurnID, "other-approval"},
				} {
					if err := s.ResolveBackgroundPermission(context.Background(), identity.session, identity.turn, identity.approval, allowed); err == nil {
						t.Fatalf("mismatched identity resolved a permission: %#v", identity)
					}
				}
				canceled, cancel := context.WithCancel(context.Background())
				cancel()
				if err := s.ResolveBackgroundPermission(canceled, ask.SessionID, ask.TurnID, ask.ApprovalID, allowed); !errors.Is(err, context.Canceled) {
					t.Fatalf("canceled resolver error = %v", err)
				}
				synctest.Wait()
				backgroundTestPending(t, s, 1)
				select {
				case got := <-result:
					t.Fatalf("rejected response unblocked permission with %v", got)
				default:
				}

				if err := s.ResolveBackgroundPermission(context.Background(), ask.SessionID, ask.TurnID, ask.ApprovalID, allowed); err != nil {
					t.Fatalf("first permission response: %v", err)
				}
				if err := s.ResolveBackgroundPermission(context.Background(), ask.SessionID, ask.TurnID, ask.ApprovalID, !allowed); err == nil {
					t.Fatal("second permission response was accepted")
				}
				if got := backgroundTestReceive(t, result); got != allowed {
					t.Fatalf("OnToolAsk() = %v, want %v", got, allowed)
				}
				resolved := backgroundTestReceive(t, events)
				if resolved.Kind != "permission_resolved" || resolved.Status != status ||
					resolved.ApprovalID != ask.ApprovalID || resolved.TurnID != ask.TurnID || resolved.Sequence <= ask.Sequence {
					t.Fatalf("invalid permission resolution: %#v", resolved)
				}
				backgroundTestPending(t, s, 0)
			})
		})
	}
}

func TestBackgroundQuestionAnswerValidationAndIdentity(t *testing.T) {
	for _, dismissed := range []bool{false, true} {
		name, status := "answered", "answered"
		if dismissed {
			name, status = "dismissed", "dismissed"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := NewChatService(ChatDependencies{})
				events, unsubscribe := s.SubscribeBackgroundTurns(8)
				defer unsubscribe()
				stream, finish := s.backgroundStreamWithIdentity("session", "turn", "wire-request")
				defer finish(ChatResponse{}, nil)
				result := backgroundTestAskQuestion(stream, context.Background(), backgroundTestRequest("question"))
				ask := backgroundTestReceive(t, events)
				if ask.Kind != "user_question" || ask.Question == nil || ask.Question.ID != "question" || ask.Question.SessionID != "session" {
					t.Fatalf("invalid question ask: %#v", ask)
				}
				for _, identity := range []struct{ session, turn, question string }{
					{"other-session", ask.TurnID, ask.Question.ID},
					{ask.SessionID, "other-turn", ask.Question.ID},
					{ask.SessionID, ask.TurnID, "other-question"},
				} {
					if err := s.ResolveBackgroundQuestion(context.Background(), identity.session, identity.turn, identity.question, backgroundTestAnswer()); err == nil {
						t.Fatalf("mismatched identity resolved a question: %#v", identity)
					}
				}
				canceled, cancel := context.WithCancel(context.Background())
				cancel()
				if err := s.ResolveBackgroundQuestion(canceled, ask.SessionID, ask.TurnID, ask.Question.ID, backgroundTestAnswer()); !errors.Is(err, context.Canceled) {
					t.Fatalf("canceled resolver error = %v", err)
				}
				for _, invalid := range []domaininteraction.Answer{
					{Answers: map[string]string{"color": "blue"}},
					{Answers: map[string]string{"color": "orange", "details": "details"}},
					{Answers: map[string]string{"color": "blue", "unknown": "details"}},
					{Answers: map[string]string{"color": "blue", "details": "  "}},
				} {
					if err := s.ResolveBackgroundQuestion(context.Background(), ask.SessionID, ask.TurnID, ask.Question.ID, invalid); err == nil {
						t.Fatalf("invalid answer was accepted: %#v", invalid)
					}
					if pending := s.ListBackgroundQuestions(ask.SessionID); len(pending) != 1 || pending[0].ID != ask.Question.ID {
						t.Fatalf("invalid answer removed the pending question: %#v", pending)
					}
				}
				synctest.Wait()
				backgroundTestPending(t, s, 1)
				select {
				case got := <-result:
					t.Fatalf("rejected response unblocked question: %#v", got)
				default:
				}

				answer := backgroundTestAnswer()
				if dismissed {
					answer = domaininteraction.Answer{Cancelled: true}
				}
				if err := s.ResolveBackgroundQuestion(context.Background(), ask.SessionID, ask.TurnID, ask.Question.ID, answer); err != nil {
					t.Fatalf("first question response: %v", err)
				}
				if !dismissed {
					answer.Answers["color"] = "mutated after resolving"
				}
				if err := s.ResolveBackgroundQuestion(context.Background(), ask.SessionID, ask.TurnID, ask.Question.ID, domaininteraction.Answer{Cancelled: true}); err == nil {
					t.Fatal("second question response was accepted")
				}
				got := backgroundTestReceive(t, result)
				if got.err != nil || got.answer.Cancelled != dismissed || (!dismissed && !reflect.DeepEqual(got.answer, backgroundTestAnswer())) {
					t.Fatalf("OnUserQuestion() = %#v", got)
				}
				resolved := backgroundTestReceive(t, events)
				if resolved.Kind != "user_question_resolved" || resolved.Status != status || resolved.Question == nil ||
					resolved.Question.ID != ask.Question.ID || resolved.TurnID != ask.TurnID || resolved.Sequence <= ask.Sequence {
					t.Fatalf("invalid question resolution: %#v", resolved)
				}
				backgroundTestPending(t, s, 0)
				if pending := s.ListBackgroundQuestions(ask.SessionID); len(pending) != 0 {
					t.Fatalf("answered question remains listed: %#v", pending)
				}
			})
		})
	}
}

func TestBackgroundListQuestionsUsesTurnIdentityAndDeepCopies(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewChatService(ChatDependencies{})
		events, unsubscribe := s.SubscribeBackgroundTurns(16)
		defer unsubscribe()
		type pendingQuestion struct {
			session, turn string
			request       domaininteraction.Request
			result        <-chan backgroundTestQuestionResult
		}
		pending := []pendingQuestion{
			{session: "session", turn: "turn-z", request: backgroundTestRequest("question-z")},
			{session: "other-session", turn: "other-turn", request: backgroundTestRequest("question-other")},
			{session: "session", turn: "turn-a", request: backgroundTestRequest("question-a")},
		}
		for i := range pending {
			p := &pending[i]
			stream, finish := s.backgroundStreamWithIdentity(p.session, p.turn, "wire-request")
			defer finish(ChatResponse{}, nil)
			p.result = backgroundTestAskQuestion(stream, context.Background(), p.request)
			if ask := backgroundTestReceive(t, events); ask.Kind != "user_question" || ask.Question == nil || ask.Question.ID != p.request.ID {
				t.Fatalf("invalid question ask: %#v", ask)
			}
		}
		synctest.Wait()
		want := []domaininteraction.Request{backgroundTestRequest("question-a"), backgroundTestRequest("question-z")}
		want[0].SessionID, want[0].RequestID = "session", "turn-a"
		want[1].SessionID, want[1].RequestID = "session", "turn-z"
		listed := s.ListBackgroundQuestions("session")
		if !reflect.DeepEqual(listed, want) {
			t.Fatalf("ListBackgroundQuestions() = %#v, want %#v", listed, want)
		}
		listed[0].Title = "changed listed title"
		listed[0].Questions[0].Options[0] = "changed listed option"
		listed[0].Questions[0].Prompt = "changed listed prompt"
		listed[0].Questions[1] = domaininteraction.Question{ID: "changed-question"}
		for i := range pending {
			pending[i].request.Questions[0].Options[0] = "changed source option"
			pending[i].request.Questions[0].Prompt = "changed source prompt"
		}
		if again := s.ListBackgroundQuestions("session"); !reflect.DeepEqual(again, want) {
			t.Fatalf("pending questions changed through a returned/source slice: %#v", again)
		}
		other := s.ListBackgroundQuestions("other-session")
		if len(other) != 1 || other[0].ID != "question-other" || other[0].RequestID != "other-turn" {
			t.Fatalf("question listing crossed sessions or lost turn identity: %#v", other)
		}
		if got := s.ListBackgroundQuestions("missing-session"); len(got) != 0 {
			t.Fatalf("missing session has pending questions: %#v", got)
		}
		for _, p := range pending {
			if err := s.ResolveBackgroundQuestion(context.Background(), p.session, p.turn, p.request.ID, domaininteraction.Answer{Cancelled: true}); err != nil {
				t.Fatal(err)
			}
			if got := backgroundTestReceive(t, p.result); got.err != nil || !got.answer.Cancelled {
				t.Fatalf("question dismissal = %#v", got)
			}
		}
		backgroundTestPending(t, s, 0)
	})
}

func TestBackgroundSubscriptionReplaysPendingIdentityAndDeepCopies(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewChatService(ChatDependencies{})
		first, stopFirst := s.SubscribeBackgroundTurns(16)
		defer stopFirst()
		second, stopSecond := s.SubscribeBackgroundTurns(16)
		defer stopSecond()
		stream, finish := s.backgroundStreamWithIdentity("session", "turn", "wire-request")
		defer finish(ChatResponse{}, nil)
		stream.OnRunStarted(domainagentrun.Run{ID: "run", SessionID: "session"})
		backgroundTestReceive(t, first)
		backgroundTestReceive(t, second)
		permission := llm.ToolPermissionRequest{ToolCallID: "tool-call", Name: "write_file", Arguments: `{"path":"notes.txt"}`, Mode: "ask"}
		permissionResult := make(chan bool, 1)
		go func() { permissionResult <- stream.OnToolAsk(permission) }()
		permissionFirst, permissionSecond := backgroundTestReceive(t, first), backgroundTestReceive(t, second)
		questionResult := backgroundTestAskQuestion(stream, context.Background(), backgroundTestRequest("question"))
		questionFirst, questionSecond := backgroundTestReceive(t, first), backgroundTestReceive(t, second)
		if permissionFirst.Kind != "permission_ask" || permissionFirst.Permission == nil || permissionSecond.Permission == nil ||
			questionFirst.Kind != "user_question" || questionFirst.Question == nil || questionSecond.Question == nil {
			t.Fatal("background stream did not publish both pending interactions")
		}
		wantPermission := permissionSecond
		wantPermission.Permission = &permission
		wantQuestion := questionSecond
		question := backgroundTestRequest("question")
		question.SessionID = "session"
		wantQuestion.Question = &question
		if wantPermission.ApprovalID == "" || wantPermission.Sequence == 0 || wantQuestion.Sequence <= wantPermission.Sequence ||
			wantPermission.RunID != "run" || wantQuestion.RunID != "run" ||
			!reflect.DeepEqual(permissionFirst, wantPermission) || !reflect.DeepEqual(questionFirst, wantQuestion) {
			t.Fatalf("invalid live interaction identities: permission=%#v question=%#v", permissionFirst, questionFirst)
		}
		permissionFirst.Permission.Name = "changed live tool"
		permissionFirst.Permission.Arguments = "changed live arguments"
		questionFirst.Question.Title = "changed live title"
		questionFirst.Question.Questions[0].Prompt = "changed live prompt"
		questionFirst.Question.Questions[0].Options[0] = "changed live option"
		if !reflect.DeepEqual(permissionSecond, wantPermission) || !reflect.DeepEqual(questionSecond, wantQuestion) {
			t.Fatal("live subscribers share mutable interaction payloads")
		}

		// Even a requested buffer of one must fit both replayed pending events.
		replayed, stopReplay := s.SubscribeBackgroundTurns(1)
		defer stopReplay()
		permissionReplay, questionReplay := backgroundTestReceive(t, replayed), backgroundTestReceive(t, replayed)
		if !reflect.DeepEqual(permissionReplay, wantPermission) || !reflect.DeepEqual(questionReplay, wantQuestion) {
			t.Fatalf("pending replay changed identity, sequence, order, or payload: permission=%#v question=%#v", permissionReplay, questionReplay)
		}
		permissionReplay.Permission.Name = "changed replay tool"
		questionReplay.Question.Questions[0].Options[0] = "changed replay option"
		questionReplay.Question.Questions[1] = domaininteraction.Question{ID: "changed replay question"}
		again, stopAgain := s.SubscribeBackgroundTurns(1)
		defer stopAgain()
		if got := backgroundTestReceive(t, again); !reflect.DeepEqual(got, wantPermission) {
			t.Fatalf("replayed permission mutated the pending event: %#v", got)
		}
		if got := backgroundTestReceive(t, again); !reflect.DeepEqual(got, wantQuestion) {
			t.Fatalf("replayed question mutated the pending event: %#v", got)
		}
		if err := s.ResolveBackgroundPermission(context.Background(), "session", "turn", wantPermission.ApprovalID, true); err != nil {
			t.Fatal(err)
		}
		if err := s.ResolveBackgroundQuestion(context.Background(), "session", "turn", "question", backgroundTestAnswer()); err != nil {
			t.Fatal(err)
		}
		if !backgroundTestReceive(t, permissionResult) {
			t.Fatal("replayed permission did not resolve the original callback")
		}
		if got := backgroundTestReceive(t, questionResult); got.err != nil || !reflect.DeepEqual(got.answer, backgroundTestAnswer()) {
			t.Fatalf("replayed question did not resolve the original callback: %#v", got)
		}
		backgroundTestPending(t, s, 0)
		empty, stopEmpty := s.SubscribeBackgroundTurns(1)
		defer stopEmpty()
		select {
		case event := <-empty:
			t.Fatalf("resolved interaction was replayed: %#v", event)
		default:
		}
	})
}

func TestBackgroundInteractionsCleanUpOnCancellationAndTimeout(t *testing.T) {
	for _, tc := range []struct {
		name           string
		question       bool
		cancelQuestion bool
		timeout        bool
	}{
		{name: "permission_canceled"},
		{name: "question_turn_canceled", question: true},
		{name: "question_context_canceled", question: true, cancelQuestion: true},
		{name: "permission_timed_out", timeout: true},
		{name: "question_timed_out", question: true, timeout: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := NewChatService(ChatDependencies{})
				events, unsubscribe := s.SubscribeBackgroundTurns(8)
				defer unsubscribe()
				turnCtx, cancelTurn := context.WithCancel(context.Background())
				defer cancelTurn()
				questionCtx, cancelQuestion := context.WithCancel(context.Background())
				defer cancelQuestion()
				stream, finish := s.backgroundStreamForContext(turnCtx, "session", "turn", "wire-request")
				defer finish(ChatResponse{}, nil)
				var questionResult <-chan backgroundTestQuestionResult
				permissionResult := make(chan bool, 1)
				if tc.question {
					questionResult = backgroundTestAskQuestion(stream, questionCtx, backgroundTestRequest("question"))
				} else {
					go func() { permissionResult <- stream.OnToolAsk(llm.ToolPermissionRequest{Name: "write_file"}) }()
				}
				ask := backgroundTestReceive(t, events)
				synctest.Wait() // The real five-minute interaction timer is now armed.
				backgroundTestPending(t, s, 1)
				wantErr, wantStatus := context.Canceled, "canceled"
				if tc.timeout {
					wantErr, wantStatus = context.DeadlineExceeded, "timed_out"
					time.Sleep(5 * time.Minute) // synctest advances virtual time only.
				} else if tc.cancelQuestion {
					cancelQuestion()
				} else {
					cancelTurn()
				}
				if tc.question {
					got := backgroundTestReceive(t, questionResult)
					if !got.answer.Cancelled || !errors.Is(got.err, wantErr) {
						t.Fatalf("terminated question = %#v, want cancellation and %v", got, wantErr)
					}
				} else if backgroundTestReceive(t, permissionResult) {
					t.Fatal("canceled or timed-out permission was allowed")
				}
				resolved := backgroundTestReceive(t, events)
				wantKind := "permission_resolved"
				if tc.question {
					wantKind = "user_question_resolved"
				}
				if resolved.Kind != wantKind || resolved.Status != wantStatus || resolved.Sequence <= ask.Sequence {
					t.Fatalf("invalid termination event: %#v", resolved)
				}
				backgroundTestPending(t, s, 0)
				if pending := s.ListBackgroundQuestions("session"); len(pending) != 0 {
					t.Fatalf("terminated question remains listed: %#v", pending)
				}
				if tc.question {
					if err := s.ResolveBackgroundQuestion(context.Background(), "session", "turn", "question", backgroundTestAnswer()); err == nil {
						t.Fatal("terminated question accepted a late response")
					}
				} else if err := s.ResolveBackgroundPermission(context.Background(), "session", "turn", ask.ApprovalID, true); err == nil {
					t.Fatal("terminated permission accepted a late response")
				}
				replay, stopReplay := s.SubscribeBackgroundTurns(1)
				defer stopReplay()
				select {
				case event := <-replay:
					t.Fatalf("terminated interaction was replayed: %#v", event)
				default:
				}
			})
		})
	}
}

func TestBackgroundCloseCancelsOwnedWorkersAndClearsInteractions(t *testing.T) {
	for _, question := range []bool{false, true} {
		name := "permission"
		if question {
			name = "question"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var calls, flushes atomic.Int32
				exited := make(chan struct{})
				contextError := make(chan error, 1)
				permissionResult := make(chan bool, 1)
				questionResult := make(chan backgroundTestQuestionResult, 1)
				s, queue := newContinuationChat(func(ctx context.Context, cmd generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
					calls.Add(1)
					defer close(exited)
					if question {
						// The worker's turn context must also cancel a caller with an independent context.
						answer, err := cmd.Stream.OnUserQuestion(context.Background(), backgroundTestRequest("question"))
						questionResult <- backgroundTestQuestionResult{answer: answer, err: err}
					} else {
						permissionResult <- cmd.Stream.OnToolAsk(llm.ToolPermissionRequest{Name: "write_file"})
					}
					contextError <- ctx.Err()
					return generationresult.GenerationResponse{}, ctx.Err()
				})
				s.dependencies.FlushPersistence = func(context.Context) error {
					select {
					case <-exited:
					default:
						return errors.New("persistence flushed before the owned worker exited")
					}
					flushes.Add(1)
					return nil
				}
				events, unsubscribe := s.SubscribeBackgroundTurns(16)
				defer unsubscribe()
				if err := s.EnqueueQueuedUserTurn(context.Background(), "parent", "queued-request", "continue with this change"); err != nil {
					t.Fatal(err)
				}
				ask := backgroundTestReceive(t, events)
				wantAsk, wantResolved := "permission_ask", "permission_resolved"
				if question {
					wantAsk, wantResolved = "user_question", "user_question_resolved"
				}
				if ask.Kind != wantAsk || ask.SessionID != "parent" || ask.RequestID != "queued-request" || ask.TurnID == "" {
					t.Fatalf("owned worker did not enter background interaction: %#v", ask)
				}
				synctest.Wait()
				backgroundTestPending(t, s, 1)
				closeCtx, cancelClose := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancelClose()
				if err := s.Close(closeCtx); err != nil {
					t.Fatalf("Close() did not cancel and wait for its worker: %v", err)
				}
				if err := backgroundTestReceive(t, contextError); !errors.Is(err, context.Canceled) {
					t.Fatalf("owned worker context error = %v, want context.Canceled", err)
				}
				if question {
					got := backgroundTestReceive(t, questionResult)
					if !got.answer.Cancelled || !errors.Is(got.err, context.Canceled) {
						t.Fatalf("Close() did not cancel question callback: %#v", got)
					}
					if err := s.ResolveBackgroundQuestion(context.Background(), "parent", ask.TurnID, "question", backgroundTestAnswer()); err == nil {
						t.Fatal("closed worker accepted a late question response")
					}
				} else {
					if backgroundTestReceive(t, permissionResult) {
						t.Fatal("Close() allowed a pending permission")
					}
					if err := s.ResolveBackgroundPermission(context.Background(), "parent", ask.TurnID, ask.ApprovalID, true); err == nil {
						t.Fatal("closed worker accepted a late permission response")
					}
				}
				backgroundTestPending(t, s, 0)
				if pending := s.ListBackgroundQuestions("parent"); len(pending) != 0 {
					t.Fatalf("Close() leaked pending questions: %#v", pending)
				}
				resolved, completed := backgroundTestReceive(t, events), backgroundTestReceive(t, events)
				if resolved.Kind != wantResolved || resolved.Status != "canceled" || completed.Kind != "completed" || completed.Status != "paused" ||
					resolved.TurnID != ask.TurnID || completed.TurnID != ask.TurnID || completed.Sequence <= resolved.Sequence {
					t.Fatalf("Close() terminal events: resolved=%#v completed=%#v", resolved, completed)
				}
				if flushes.Load() != 1 || !queue.HasPending("parent") {
					t.Fatalf("Close() flush count = %d, canceled input retained = %v", flushes.Load(), queue.HasPending("parent"))
				}
				s.schedulePendingTurn("parent")
				synctest.Wait()
				s.pendingWakeMu.Lock()
				workers, closed := len(s.pendingWakes), s.backgroundClosed
				s.pendingWakeMu.Unlock()
				if !closed || workers != 0 || calls.Load() != 1 {
					t.Fatalf("Close() leaked or restarted an owned worker: closed=%v workers=%d calls=%d", closed, workers, calls.Load())
				}
				replay, stopReplay := s.SubscribeBackgroundTurns(1)
				defer stopReplay()
				select {
				case event := <-replay:
					t.Fatalf("Close() left an interaction to replay: %#v", event)
				default:
				}
			})
		})
	}
}
