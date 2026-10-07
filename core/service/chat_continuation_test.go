package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"myai/core/adapter/chat/pendinginput"
	generationcommand "myai/core/application/chat/generation/command"
	generationresult "myai/core/application/chat/generation/result"
	domainagentrun "myai/core/domain/agentrun"
	domainsubagent "myai/core/domain/subagent"
	"myai/core/llm"
	"myai/core/session"
)

type continuationTestControl struct {
	mu      sync.Mutex
	paused  bool
	deleted bool
}

func (c *continuationTestControl) ContinuationAllowed(context.Context, string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.paused && !c.deleted, nil
}
func (c *continuationTestControl) SetContinuationPaused(_ context.Context, _ string, paused bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.paused = paused
	return nil
}

type continuationGenerator func(context.Context, generationcommand.GenerationTask) (generationresult.GenerationResponse, error)

func (g continuationGenerator) Generate(ctx context.Context, cmd generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
	return g(ctx, cmd)
}

func newContinuationChat(g continuationGenerator) (*ChatService, *pendinginput.Queue) {
	queue := pendinginput.NewQueue()
	current := &session.Session{ID: "parent", Model: "test"}
	return NewChatService(ChatDependencies{
		Models: autoPlanModelRegistry{}, TurnInputQueue: queue, GenerationTasks: g,
		MessageCommands: &recordingAutoPlanMessages{current: current},
	}), queue
}

func childCompletion(id string) domainsubagent.AgentMessage {
	return domainsubagent.AgentMessage{ID: id, SourceTaskID: "child", RecipientAgentID: "parent", Kind: domainsubagent.AgentMessageKindTaskResult,
		Content: "child result " + id, Trigger: domainsubagent.AgentMessageTriggerTurn, Status: domainsubagent.AgentMessagePending, CreatedAt: time.Now()}
}

func waitContinuationWorkers(t *testing.T, s *ChatService) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		s.pendingWakeMu.Lock()
		n := len(s.pendingWakes)
		s.pendingWakeMu.Unlock()
		if n == 0 {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("continuation worker did not exit")
		case <-tick.C:
		}
	}
}

func TestPendingTurnWaitsForLockAndCoalescesCompletions(t *testing.T) {
	var calls atomic.Int32
	s, queue := newContinuationChat(func(_ context.Context, cmd generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
		calls.Add(1)
		return generationresult.GenerationResponse{SessionID: cmd.Session.ID}, nil
	})
	unlock, _ := s.lockSessionOperation(context.Background(), "parent")
	if err := s.EnqueueTurnInputAgentMessage("parent", childCompletion("one")); err != nil {
		t.Fatal(err)
	}
	if err := s.EnqueueTurnInputAgentMessage("parent", childCompletion("two")); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("generated while foreground owns lock")
	}
	unlock()
	waitContinuationWorkers(t, s)
	if calls.Load() != 1 || queue.HasPending("parent") {
		t.Fatalf("calls=%d pending=%v", calls.Load(), queue.HasPending("parent"))
	}
}

func TestForegroundConsumptionPreventsDuplicateBackgroundAnswer(t *testing.T) {
	var calls atomic.Int32
	s, queue := newContinuationChat(func(context.Context, generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
		calls.Add(1)
		return generationresult.GenerationResponse{}, nil
	})
	unlock, _ := s.lockSessionOperation(context.Background(), "parent")
	if err := s.EnqueueTurnInputAgentMessage("parent", childCompletion("one")); err != nil {
		t.Fatal(err)
	}
	for _, item := range queue.DrainIdentified("parent") {
		if err := queue.Acknowledge(item.ID); err != nil {
			t.Fatal(err)
		}
	}
	unlock()
	waitContinuationWorkers(t, s)
	if calls.Load() != 0 {
		t.Fatal("already consumed result produced another answer")
	}
}

func TestPauseCancelsBackgroundTurnAndRetainsResultAcrossRestart(t *testing.T) {
	started := make(chan struct{})
	control := &continuationTestControl{}
	s, queue := newContinuationChat(func(ctx context.Context, cmd generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
		cmd.Stream.OnRunStarted(domainagentrun.Run{ID: "run-1", SessionID: "parent"})
		close(started)
		<-ctx.Done()
		return generationresult.GenerationResponse{}, ctx.Err()
	})
	s.dependencies.ContinuationControl = control
	events, unsubscribe := s.SubscribeBackgroundTurns(16)
	defer unsubscribe()
	if err := s.EnqueueTurnInputAgentMessage("parent", childCompletion("one")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("not started")
	}
	if err := s.PauseSessionContinuations(context.Background(), "parent"); err != nil {
		t.Fatal(err)
	}
	waitContinuationWorkers(t, s)
	if !queue.HasPending("parent") {
		t.Fatal("canceled turn lost result")
	}
	var terminal BackgroundTurnEvent
	for len(events) > 0 {
		terminal = <-events
	}
	if terminal.Kind != "completed" || terminal.Status != "paused" {
		t.Fatalf("terminal=%#v", terminal)
	}
	// A fresh service instance must honor the durable pause, not just its map.
	restarted := NewChatService(ChatDependencies{ContinuationControl: control})
	if _, _, err := restarted.beginContinuation(context.Background(), "parent"); !errors.Is(err, ErrContinuationPaused) {
		t.Fatalf("restart ignored pause: %v", err)
	}
	if err := restarted.resumeContinuations(context.Background(), "parent"); err != nil {
		t.Fatal(err)
	}
	_, finish, err := restarted.beginContinuation(context.Background(), "parent")
	if err != nil {
		t.Fatal(err)
	}
	finish()
}

func TestLateCompletionsDoNotRestartPausedOrDeletedSession(t *testing.T) {
	for _, deleted := range []bool{false, true} {
		var calls atomic.Int32
		s, queue := newContinuationChat(func(context.Context, generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
			calls.Add(1)
			return generationresult.GenerationResponse{}, nil
		})
		s.dependencies.ContinuationControl = &continuationTestControl{paused: !deleted, deleted: deleted}
		if err := s.EnqueueTurnInputAgentMessage("parent", childCompletion("late")); err != nil {
			t.Fatal(err)
		}
		waitContinuationWorkers(t, s)
		if calls.Load() != 0 || !queue.HasPending("parent") {
			t.Fatal("late completion ran or disappeared")
		}
	}
}

func TestCompletionDuringBackgroundGenerationSchedulesFollowingTurn(t *testing.T) {
	var calls atomic.Int32
	var s *ChatService
	s, _ = newContinuationChat(func(_ context.Context, cmd generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
		if calls.Add(1) == 1 {
			if err := s.EnqueueTurnInputAgentMessage("parent", childCompletion("later")); err != nil {
				return generationresult.GenerationResponse{}, err
			}
		}
		return generationresult.GenerationResponse{SessionID: cmd.Session.ID}, nil
	})
	if err := s.EnqueueTurnInputAgentMessage("parent", childCompletion("first")); err != nil {
		t.Fatal(err)
	}
	waitContinuationWorkers(t, s)
	if calls.Load() != 2 {
		t.Fatalf("late wake lost, calls=%d", calls.Load())
	}
}

func TestBackgroundStreamIdentitySequenceAndSlowSubscriber(t *testing.T) {
	s := NewChatService(ChatDependencies{})
	fast, unsubscribe := s.SubscribeBackgroundTurns(16)
	defer unsubscribe()
	slow, stopSlow := s.SubscribeBackgroundTurns(1)
	defer stopSlow()
	stream, finish := s.backgroundStream("parent")
	stream.OnRunStarted(domainagentrun.Run{ID: "run", SessionID: "parent"})
	stream.OnAnswer("hello")
	stream.OnRunCompleted(domainagentrun.Run{ID: "run", Status: domainagentrun.StatusSucceeded})
	finish(ChatResponse{Result: llm.ChatResult{Content: "hello"}}, nil)
	var previous uint64
	for i := 0; i < 3; i++ {
		e := <-fast
		if e.Sequence <= previous || e.SessionID != "parent" || e.RunID != "run" || e.TurnID != stream.CorrelationID {
			t.Fatalf("invalid event: %#v", e)
		}
		previous = e.Sequence
	}
	<-slow
	if _, open := <-slow; open {
		t.Fatal("slow subscriber should require resync")
	}
	second, _ := s.backgroundStream("parent")
	if second.CorrelationID == stream.CorrelationID {
		t.Fatal("continuation reused turn identity")
	}
}
