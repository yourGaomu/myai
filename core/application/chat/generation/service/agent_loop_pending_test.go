package service

import (
	"context"
	"errors"
	"testing"

	"myai/core/adapter/chat/pendinginput"
	generationcommand "myai/core/application/chat/generation/command"
	"myai/core/contextmgr"
	modelport "myai/core/port/model"
	"myai/core/session"
)

type pendingContextProvider struct{}

func (pendingContextProvider) Snapshot(current *session.Session) contextmgr.Snapshot {
	return contextmgr.BuildSnapshot(current.Messages, current.Summary, current.CompactedMessages, current.ContextWindowK)
}

type pendingModel struct {
	responses []modelport.ChatResult
	errors    []error
	calls     int
}

func (model *pendingModel) Generate(context.Context, modelport.GenerateRequest) (modelport.ChatResult, error) {
	index := model.calls
	model.calls++
	var response modelport.ChatResult
	if index < len(model.responses) {
		response = model.responses[index]
	}
	if index < len(model.errors) && model.errors[index] != nil {
		return modelport.ChatResult{}, model.errors[index]
	}
	if response.Content == "" {
		response.Content = "ok"
	}
	return response, nil
}

func TestAgentLoopAcknowledgesPendingMessageAfterSuccessfulTurn(t *testing.T) {
	queue := pendinginput.NewQueue()
	acknowledger := &pendingTestAcknowledger{}
	queue.SetAcknowledger(acknowledger)
	if err := queue.EnqueueIdentified("session-1", "message-1", "child result"); err != nil {
		t.Fatal(err)
	}
	model := &pendingModel{responses: []modelport.ChatResult{{Content: "first"}, {Content: "parent answer"}}}
	service := AgentLoopService{Contexts: pendingContextProvider{}, PendingInput: queue}
	_, err := service.Run(context.Background(), generationcommand.Run{
		Model: model, Session: &session.Session{ID: "session-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(acknowledger.ids) != 1 || acknowledger.ids[0] != "message-1" {
		t.Fatalf("pending message was not acknowledged after success: %#v", acknowledger.ids)
	}
	if queue.HasPending("session-1") {
		t.Fatal("successful turn left pending input queued")
	}
}

func TestAgentLoopRequeuesPendingMessageWhenTurnFails(t *testing.T) {
	queue := pendinginput.NewQueue()
	if err := queue.EnqueueIdentified("session-1", "message-1", "child result"); err != nil {
		t.Fatal(err)
	}
	model := &pendingModel{
		responses: []modelport.ChatResult{{Content: "first"}},
		errors:    []error{nil, errors.New("model unavailable")},
	}
	service := AgentLoopService{Contexts: pendingContextProvider{}, PendingInput: queue}
	_, err := service.Run(context.Background(), generationcommand.Run{
		Model: model, Session: &session.Session{ID: "session-1"},
	})
	if err == nil {
		t.Fatal("expected model failure")
	}
	items := queue.DrainIdentified("session-1")
	if len(items) != 1 || items[0].ID != "message-1" || items[0].Content != "child result" {
		t.Fatalf("failed turn did not requeue pending message: %#v", items)
	}
}

type pendingTestAcknowledger struct {
	ids []string
}

func (acknowledger *pendingTestAcknowledger) Acknowledge(messageID string) error {
	acknowledger.ids = append(acknowledger.ids, messageID)
	return nil
}
