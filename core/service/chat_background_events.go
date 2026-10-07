package service

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"
	domainagentrun "myai/core/domain/agentrun"
	"myai/core/llm"
)

// BackgroundTurnEvent is independent of a foreground request or connection.
// Sequence is per TurnID. Run history is the recovery source after disconnect.
type BackgroundTurnEvent struct {
	TurnID    string
	SessionID string
	RunID     string
	Sequence  uint64
	Kind      string
	Status    string
	Content   string
	Reasoning string
	Error     string
	Run       *domainagentrun.Run
	Event     *domainagentrun.Event
}

func (s *ChatService) SubscribeBackgroundTurns(buffer int) (<-chan BackgroundTurnEvent, func()) {
	if buffer < 1 {
		buffer = 1
	}
	if buffer > 1024 {
		buffer = 1024
	}
	ch := make(chan BackgroundTurnEvent, buffer)
	s.backgroundEventsMu.Lock()
	if s.backgroundSubscribers == nil {
		s.backgroundSubscribers = make(map[chan BackgroundTurnEvent]struct{})
	}
	s.backgroundSubscribers[ch] = struct{}{}
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

func (s *ChatService) publishBackgroundTurn(event BackgroundTurnEvent) {
	s.backgroundEventsMu.Lock()
	defer s.backgroundEventsMu.Unlock()
	for ch := range s.backgroundSubscribers {
		select {
		case ch <- event:
		default:
			// Never silently lose a terminal event or block model execution on a
			// slow connection. The consumer must reconnect and resync history.
			delete(s.backgroundSubscribers, ch)
			close(ch)
		}
	}
}

func (s *ChatService) backgroundStream(sessionID string) (llm.ChatStreamHandler, func(ChatResponse, error)) {
	turnID := uuid.NewString()
	var mu sync.Mutex
	var sequence uint64
	var runID string
	var finishedRun *domainagentrun.Run
	emit := func(event BackgroundTurnEvent) {
		// Callbacks can arrive concurrently from parallel tools.
		mu.Lock()
		defer mu.Unlock()
		if event.Run != nil {
			runID = event.Run.ID
		}
		sequence++
		event.TurnID, event.SessionID, event.RunID, event.Sequence = turnID, sessionID, runID, sequence
		s.publishBackgroundTurn(event)
	}
	stream := llm.ChatStreamHandler{
		CorrelationID:  turnID,
		OnRunStarted:   func(run domainagentrun.Run) { emit(BackgroundTurnEvent{Kind: "started", Status: "running", Run: &run}) },
		OnRunEvent:     func(event domainagentrun.Event) { emit(BackgroundTurnEvent{Kind: "run_event", Event: &event}) },
		OnRunCompleted: func(run domainagentrun.Run) { mu.Lock(); finishedRun = &run; mu.Unlock() },
		OnAnswer:       func(text string) { emit(BackgroundTurnEvent{Kind: "delta", Content: text}) },
		OnReasoning:    func(text string) { emit(BackgroundTurnEvent{Kind: "delta", Reasoning: text}) },
		// Unattended continuations cannot obtain interactive permission. The
		// executor's normal deny behavior remains in force when OnToolAsk is nil.
	}
	return stream, func(response ChatResponse, err error) {
		status, errorText := "succeeded", ""
		if err != nil {
			status, errorText = "failed", err.Error()
			if errors.Is(err, context.Canceled) {
				status = "paused"
			}
		}
		mu.Lock()
		run := finishedRun
		mu.Unlock()
		emit(BackgroundTurnEvent{Kind: "completed", Status: status, Error: errorText, Content: response.Result.Content, Reasoning: response.Result.Reasoning, Run: run})
	}
}
