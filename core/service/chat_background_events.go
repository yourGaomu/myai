package service

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	domainagentrun "myai/core/domain/agentrun"
	domaininteraction "myai/core/domain/interaction"
	domaintool "myai/core/domain/tool"
	"myai/core/llm"
	intentport "myai/core/port/intent"
)

// BackgroundTurnEvent belongs to one background turn, independent of a client.
type BackgroundTurnEvent struct {
	TurnID, RequestID, SessionID, RunID     string
	MessageID                               string
	Sequence                                uint64
	Kind, Status, Content, Reasoning, Error string
	Run                                     *domainagentrun.Run
	Event                                   *domainagentrun.Event
	ApprovalID                              string
	Permission                              *llm.ToolPermissionRequest
	Question                                *domaininteraction.Request
	Assets                                  []domaintool.SharedAsset
	IntentTrace                             *intentport.Trace
}

type backgroundInteraction struct {
	event  BackgroundTurnEvent
	result chan backgroundInteractionResult
}
type backgroundInteractionResult struct {
	allowed bool
	answer  domaininteraction.Answer
	status  string
	err     error
}

func (s *ChatService) ListBackgroundQuestions(sessionID string) []domaininteraction.Request {
	if s == nil {
		return nil
	}
	s.backgroundEventsMu.Lock()
	defer s.backgroundEventsMu.Unlock()
	var requests []domaininteraction.Request
	for _, waiter := range s.backgroundPending {
		if waiter.event.Question == nil || (sessionID != "" && waiter.event.SessionID != sessionID) {
			continue
		}
		event := cloneBackgroundEvent(waiter.event)
		request := *event.Question
		request.SessionID, request.RequestID = event.SessionID, event.TurnID
		requests = append(requests, request)
	}
	sort.Slice(requests, func(i, j int) bool {
		if requests[i].ID != requests[j].ID {
			return requests[i].ID < requests[j].ID
		}
		if requests[i].SessionID != requests[j].SessionID {
			return requests[i].SessionID < requests[j].SessionID
		}
		return requests[i].RequestID < requests[j].RequestID
	})
	return requests
}

func interactionKey(sessionID, turnID, id string) string {
	return sessionID + "\x00" + turnID + "\x00" + id
}

func (s *ChatService) SubscribeBackgroundTurns(buffer int) (<-chan BackgroundTurnEvent, func()) {
	if buffer < 1 {
		buffer = 1
	}
	if buffer > 1024 {
		buffer = 1024
	}
	s.backgroundEventsMu.Lock()
	pending := make([]BackgroundTurnEvent, 0, len(s.backgroundPending))
	for _, waiter := range s.backgroundPending {
		pending = append(pending, cloneBackgroundEvent(waiter.event))
	}
	sort.Slice(pending, func(i, j int) bool {
		if pending[i].TurnID == pending[j].TurnID {
			return pending[i].Sequence < pending[j].Sequence
		}
		return pending[i].TurnID < pending[j].TurnID
	})
	ch := make(chan BackgroundTurnEvent, max(buffer, len(pending)))
	if s.backgroundSubscribers == nil {
		s.backgroundSubscribers = make(map[chan BackgroundTurnEvent]struct{})
	}
	s.backgroundSubscribers[ch] = struct{}{}
	for _, event := range pending {
		ch <- event
	}
	s.backgroundEventsMu.Unlock()
	return ch, func() {
		s.backgroundEventsMu.Lock()
		defer s.backgroundEventsMu.Unlock()
		if _, ok := s.backgroundSubscribers[ch]; ok {
			delete(s.backgroundSubscribers, ch)
			close(ch)
		}
	}
}

func cloneBackgroundEvent(event BackgroundTurnEvent) BackgroundTurnEvent {
	event.Assets = domaintool.CloneSharedAssets(event.Assets)
	if event.Permission != nil {
		permission := *event.Permission
		event.Permission = &permission
	}
	if event.Question != nil {
		question := *event.Question
		question.Questions = append([]domaininteraction.Question(nil), question.Questions...)
		for i := range question.Questions {
			question.Questions[i].Choices = append([]domaininteraction.Option(nil), question.Questions[i].Choices...)
		}
		event.Question = &question
	}
	return event
}

func (s *ChatService) publishBackgroundTurnLocked(event BackgroundTurnEvent) {
	for ch := range s.backgroundSubscribers {
		select {
		case ch <- cloneBackgroundEvent(event):
		default:
			delete(s.backgroundSubscribers, ch)
			close(ch)
		}
	}
}
func (s *ChatService) publishBackgroundTurn(event BackgroundTurnEvent) {
	s.backgroundEventsMu.Lock()
	defer s.backgroundEventsMu.Unlock()
	s.publishBackgroundTurnLocked(event)
}

func (s *ChatService) ResolveBackgroundPermission(ctx context.Context, sessionID, turnID, approvalID string, allowed bool) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if s == nil || sessionID == "" || turnID == "" || approvalID == "" {
		return errors.New("background permission identity is required")
	}
	key := interactionKey(sessionID, turnID, approvalID)
	s.backgroundEventsMu.Lock()
	defer s.backgroundEventsMu.Unlock()
	waiter := s.backgroundPending[key]
	if waiter == nil || waiter.event.Permission == nil {
		return errors.New("background permission is no longer pending")
	}
	delete(s.backgroundPending, key)
	status := "denied"
	if allowed {
		status = "allowed"
	}
	waiter.result <- backgroundInteractionResult{allowed: allowed, status: status}
	return nil
}

func (s *ChatService) ResolveBackgroundQuestion(ctx context.Context, sessionID, turnID, questionID string, answer domaininteraction.Answer) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if s == nil || sessionID == "" || turnID == "" || questionID == "" {
		return errors.New("background question identity is required")
	}
	key := interactionKey(sessionID, turnID, questionID)
	s.backgroundEventsMu.Lock()
	defer s.backgroundEventsMu.Unlock()
	waiter := s.backgroundPending[key]
	if waiter == nil || waiter.event.Question == nil {
		return errors.New("background question is no longer pending")
	}
	if err := domaininteraction.ValidateAnswer(*waiter.event.Question, answer); err != nil {
		return err
	}
	copyAnswer := domaininteraction.Answer{
		Cancelled: answer.Cancelled, AdoptedDefaults: answer.AdoptedDefaults,
		Structured: make(map[string]domaininteraction.QuestionAnswer, len(answer.Structured)),
	}
	for id, value := range answer.Structured {
		copyAnswer.Structured[id] = domaininteraction.QuestionAnswer{
			Selected: append([]string(nil), value.Selected...), Other: value.Other,
		}
	}
	delete(s.backgroundPending, key)
	status := "answered"
	if answer.Cancelled {
		status = "dismissed"
	}
	waiter.result <- backgroundInteractionResult{answer: copyAnswer, status: status}
	return nil
}

func (s *ChatService) awaitBackgroundInteraction(ctx context.Context, event BackgroundTurnEvent) backgroundInteractionResult {
	id := event.ApprovalID
	if event.Question != nil {
		id = event.Question.ID
	}
	key := interactionKey(event.SessionID, event.TurnID, id)
	waiter := &backgroundInteraction{event: cloneBackgroundEvent(event), result: make(chan backgroundInteractionResult, 1)}
	s.backgroundEventsMu.Lock()
	if s.backgroundPending == nil {
		s.backgroundPending = make(map[string]*backgroundInteraction)
	}
	if _, exists := s.backgroundPending[key]; exists {
		s.backgroundEventsMu.Unlock()
		return backgroundInteractionResult{err: errors.New("duplicate pending interaction"), status: "dismissed"}
	}
	s.backgroundPending[key] = waiter
	s.publishBackgroundTurnLocked(event)
	s.backgroundEventsMu.Unlock()
	timer := time.NewTimer(5 * time.Minute)
	defer timer.Stop()
	var cause error
	select {
	case result := <-waiter.result:
		return result
	case <-ctx.Done():
		cause = ctx.Err()
	case <-timer.C:
		cause = context.DeadlineExceeded
	}
	s.backgroundEventsMu.Lock()
	if s.backgroundPending[key] == waiter {
		delete(s.backgroundPending, key)
		s.backgroundEventsMu.Unlock()
		status := "canceled"
		if errors.Is(cause, context.DeadlineExceeded) {
			status = "timed_out"
		}
		return backgroundInteractionResult{answer: domaininteraction.Answer{Cancelled: true}, status: status, err: cause}
	}
	s.backgroundEventsMu.Unlock()
	return <-waiter.result
}

func (s *ChatService) backgroundStream(sessionID string) (llm.ChatStreamHandler, func(ChatResponse, error)) {
	return s.backgroundStreamForContext(context.Background(), sessionID, uuid.NewString(), "")
}
func (s *ChatService) backgroundStreamForRequest(sessionID, requestID string) (llm.ChatStreamHandler, func(ChatResponse, error)) {
	return s.backgroundStreamForContext(context.Background(), sessionID, uuid.NewString(), requestID)
}
func (s *ChatService) backgroundStreamWithIdentity(sessionID, turnID, requestID string) (llm.ChatStreamHandler, func(ChatResponse, error)) {
	return s.backgroundStreamForContext(context.Background(), sessionID, turnID, requestID)
}
func (s *ChatService) backgroundStreamForContext(ctx context.Context, sessionID, turnID, requestID string) (llm.ChatStreamHandler, func(ChatResponse, error)) {
	if turnID == "" {
		turnID = uuid.NewString()
	}
	turnCtx, cancel := context.WithCancel(ctx)
	var mu sync.Mutex
	var sequence uint64
	var runID string
	var finishedRun *domainagentrun.Run
	var assets []domaintool.SharedAsset
	identify := func(event BackgroundTurnEvent) BackgroundTurnEvent {
		if event.Run != nil {
			runID = event.Run.ID
		}
		sequence++
		event.TurnID, event.RequestID, event.SessionID, event.RunID, event.Sequence = turnID, requestID, sessionID, runID, sequence
		return event
	}
	emit := func(event BackgroundTurnEvent) {
		mu.Lock()
		defer mu.Unlock()
		s.publishBackgroundTurn(identify(event))
	}
	ask := func(event BackgroundTurnEvent, askCtx context.Context) backgroundInteractionResult {
		mu.Lock()
		event = identify(event)
		mu.Unlock()
		return s.awaitBackgroundInteraction(askCtx, event)
	}
	stream := llm.ChatStreamHandler{
		CorrelationID: turnID,
		OnIntentTrace: func(trace intentport.Trace) {
			trace.RequestBody, trace.ResponseBody = "", ""
			emit(BackgroundTurnEvent{Kind: "intent_trace", IntentTrace: &trace})
		},
		OnRunStarted:   func(run domainagentrun.Run) { emit(BackgroundTurnEvent{Kind: "started", Status: "running", Run: &run}) },
		OnRunEvent:     func(event domainagentrun.Event) { emit(BackgroundTurnEvent{Kind: "run_event", Event: &event}) },
		OnRunCompleted: func(run domainagentrun.Run) { mu.Lock(); finishedRun = &run; mu.Unlock() },
		OnAnswer:       func(text string) { emit(BackgroundTurnEvent{Kind: "delta", Content: text}) },
		OnReasoning:    func(text string) { emit(BackgroundTurnEvent{Kind: "delta", Reasoning: text}) },
		OnAsset: func(asset domaintool.SharedAsset) {
			mu.Lock()
			defer mu.Unlock()
			assets = append(assets, asset)
		},
		OnToolAsk: func(request llm.ToolPermissionRequest) bool {
			id := uuid.NewString()
			result := ask(BackgroundTurnEvent{Kind: "permission_ask", ApprovalID: id, Permission: &request}, turnCtx)
			emit(BackgroundTurnEvent{Kind: "permission_resolved", ApprovalID: id, Status: result.status})
			return result.allowed && result.err == nil
		},
		OnUserQuestion: func(questionCtx context.Context, request domaininteraction.Request) (domaininteraction.Answer, error) {
			if request.ID == "" {
				request.ID = uuid.NewString()
			}
			if request.SessionID != "" && request.SessionID != sessionID {
				return domaininteraction.Answer{}, errors.New("question session does not match background turn")
			}
			request.SessionID = sessionID
			if err := domaininteraction.ValidateRequest(request); err != nil {
				return domaininteraction.Answer{}, err
			}
			askCtx, stop := context.WithCancel(questionCtx)
			defer stop()
			unregister := context.AfterFunc(turnCtx, stop)
			defer unregister()
			result := ask(BackgroundTurnEvent{Kind: "user_question", Question: &request}, askCtx)
			emit(BackgroundTurnEvent{Kind: "user_question_resolved", Question: &request, Status: result.status})
			return result.answer, result.err
		},
	}
	return stream, func(response ChatResponse, err error) {
		cancel()
		status, errorText, messageID := "succeeded", "", response.MessageID
		if err != nil {
			status, errorText, messageID = "failed", err.Error(), ""
			if errors.Is(err, context.Canceled) {
				status = "paused"
			}
		}
		mu.Lock()
		defer mu.Unlock()
		event := BackgroundTurnEvent{
			Kind: "completed", Status: status, Error: errorText, MessageID: messageID,
			Content: response.Result.Content, Reasoning: response.Result.Reasoning, Run: finishedRun,
		}
		if err == nil {
			event.Assets = assets
		}
		s.publishBackgroundTurn(identify(event))
	}
}
