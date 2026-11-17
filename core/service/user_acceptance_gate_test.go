package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	memoryadapter "myai/core/adapter/session/memory"
	contextservice "myai/core/application/chat/context/service"
	generationcommand "myai/core/application/chat/generation/command"
	generationservice "myai/core/application/chat/generation/service"
	loadservice "myai/core/application/session/load/service"
	messageservice "myai/core/application/session/message/service"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
	"myai/core/session"
)

func TestCallerAcceptanceCapacityRejectsBeforeAppendAndReleasesAfterRetry(t *testing.T) {
	ctx := context.Background()
	memory := memoryadapter.NewStore("model")
	for index := 0; index <= maxUserAcceptanceSlots; index++ {
		if err := memory.PutSession(fmt.Sprintf("session-%d", index), "model", nil); err != nil {
			t.Fatal(err)
		}
	}
	loader := loadservice.LoadService{Memory: memory}
	model, executor := &callerAcceptanceModel{}, &callerAcceptanceExecutor{}
	failure := errors.New("journal unavailable")
	sink := &callerAcceptanceSink{err: failure}
	chat := NewChatService(ChatDependencies{
		Models: semanticModelRegistry{model: model}, UserMessages: sink,
		MessageCommands: messageservice.CommandService{Memory: memory, Loader: loader},
		GenerationTasks: generationservice.TaskService{RequestIDs: callerAcceptanceIDs{}, Generator: generationservice.AssistantGenerationService{
			Models: semanticModelRegistry{model: model}, Contexts: contextservice.SnapshotService{},
			AgentRunner:       generationservice.AgentLoopService{Sessions: memory, Contexts: contextservice.SnapshotService{}, ToolExecutor: executor},
			ResponseCommitter: generationservice.ResponseCommitService{Memory: memory},
		}},
	})
	for index := 0; index < maxUserAcceptanceSlots; index++ {
		_, err := chat.ContinueSessionStreamForMessage(ctx, fmt.Sprintf("session-%d", index), "input", domainmessage.SyntheticReasonSubagentResult, "source", "task_result", modelport.ChatStreamHandler{})
		if !errors.Is(err, failure) {
			t.Fatalf("accept session %d: %v", index, err)
		}
	}
	if len(chat.pendingUserAcceptance) != maxUserAcceptanceSlots || len(chat.userAcceptanceSlots) != maxUserAcceptanceSlots {
		t.Fatal("rejected batches were not retained within the capacity limit")
	}
	extraID := fmt.Sprintf("session-%d", maxUserAcceptanceSlots)
	before, err := memory.Snapshot(extraID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = chat.ContinueSessionStreamForMessage(ctx, extraID, "must not append", domainmessage.SyntheticReasonSubagentResult, "new-source", "task_result", modelport.ChatStreamHandler{})
	if !errors.Is(err, ErrUserAcceptanceCapacity) {
		t.Fatalf("expected admission rejection, got %v", err)
	}
	after, _ := memory.Snapshot(extraID)
	if len(before.Messages) != len(after.Messages) || len(sink.commands) != maxUserAcceptanceSlots || model.calls != 0 || executor.calls != 0 {
		t.Fatal("capacity rejection appended input or ran persistence/model/tool work")
	}
	_, err = chat.ContinueSessionStreamForMessage(ctx, "session-0", "input", domainmessage.SyntheticReasonSubagentResult, "source", "task_result", modelport.ChatStreamHandler{})
	if !errors.Is(err, failure) {
		t.Fatalf("existing pending session lost its retry slot: %v", err)
	}
	sink.err = nil
	response, err := chat.ContinueSessionStreamForMessage(ctx, "session-0", "input", domainmessage.SyntheticReasonSubagentResult, "source", "task_result", modelport.ChatStreamHandler{})
	if err != nil || response.Result.Content != "done" {
		t.Fatalf("retry did not complete: response=%+v error=%v", response, err)
	}
	if len(chat.userAcceptanceSlots) != maxUserAcceptanceSlots-1 || len(chat.pendingUserAcceptance) != maxUserAcceptanceSlots-1 {
		t.Fatal("successful retry did not release its capacity slot")
	}
	sink.err = failure
	_, err = chat.ContinueSessionStreamForMessage(ctx, extraID, "input", domainmessage.SyntheticReasonSubagentResult, "new-source", "task_result", modelport.ChatStreamHandler{})
	if !errors.Is(err, failure) {
		t.Fatalf("released slot was not available for a new session: %v", err)
	}
	chat.discardUserAcceptance(extraID)
	if _, exists := chat.userAcceptanceSlots[extraID]; exists {
		t.Fatal("explicit discard retained its capacity slot")
	}
}

type mutatingAcceptanceSink struct {
	calls int
	err   error
}

func (*mutatingAcceptanceSink) PersistUserMessage(generationcommand.PersistUserMessage) {}
func (s *mutatingAcceptanceSink) AcceptUserMessage(_ context.Context, command generationcommand.PersistUserMessage) error {
	s.calls++
	command.AppendedMessages[0].SourceID = "mutated by sink"
	command.SessionSnapshot.Messages[0].Parts[0].Text = "mutated by sink"
	return s.err
}

func TestCallerAcceptanceRetryCacheDoesNotShareSinkPayload(t *testing.T) {
	failure := errors.New("journal unavailable")
	sink := &mutatingAcceptanceSink{err: failure}
	chat := NewChatService(ChatDependencies{UserMessages: sink})
	message := domainmessage.Text(domainmessage.RoleUser, "original")
	message.SourceID = "source"
	command := generationcommand.PersistUserMessage{
		SessionID: "session", Input: "original", AppendedMessages: []domainmessage.Message{message},
		SessionSnapshot: &session.Session{ID: "session", Messages: []domainmessage.Message{message}},
	}
	if err := chat.acceptUserMessage(context.Background(), command); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		pending := chat.pendingUserAcceptance["session"]
		if pending.AppendedMessages[0].SourceID != "source" || pending.SessionSnapshot.Messages[0].Text() != "original" || command.SessionSnapshot.Messages[0].Text() != "original" {
			t.Fatal("sink mutation changed the retry cache or caller's snapshot")
		}
		if err := chat.retryUserAcceptance(context.Background(), "session"); !errors.Is(err, failure) {
			t.Fatal(err)
		}
	}
	sink.err = nil
	if err := chat.retryUserAcceptance(context.Background(), "session"); err != nil {
		t.Fatal(err)
	}
	if len(chat.pendingUserAcceptance) != 0 || len(chat.userAcceptanceSlots) != 0 {
		t.Fatal("accepted retry retained cached data")
	}
}
