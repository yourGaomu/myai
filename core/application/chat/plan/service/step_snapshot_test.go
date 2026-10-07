package service

import (
	"context"
	"fmt"
	"testing"

	generationcommand "myai/core/application/chat/generation/command"
	generationresult "myai/core/application/chat/generation/result"
	messagecommand "myai/core/application/session/message/command"
	messageresult "myai/core/application/session/message/result"
	domainmessage "myai/core/domain/message"
	agentplan "myai/core/plan"
	modelport "myai/core/port/model"
	"myai/core/session"
)

type appendingMessages struct{ current *session.Session }

func (a appendingMessages) AppendUserMessage(_ context.Context, c messagecommand.AppendUserMessage) (messageresult.Command, error) {
	m := domainmessage.Text(domainmessage.RoleUser, c.Input)
	a.current.Messages = append(a.current.Messages, m)
	return messageresult.Command{Session: a.current, AppendedMessages: []domainmessage.Message{m}, Appended: true}, nil
}

type isolatedStepGenerator struct{}

func (isolatedStepGenerator) Generate(_ context.Context, c generationcommand.GenerationTask) (generationresult.GenerationResponse, error) {
	if len(c.Session.Messages) != 2 || c.Session.Messages[0].Text() != "shared history" || c.Session.Messages[1].Text() != c.LatestInput {
		return generationresult.GenerationResponse{}, fmt.Errorf("sibling instruction leaked: %#v", c.Session.Messages)
	}
	return generationresult.GenerationResponse{}, nil
}
func TestReadyBatchIsolatesSiblingInstructions(t *testing.T) {
	current := &session.Session{ID: "session", Messages: []domainmessage.Message{domainmessage.Text(domainmessage.RoleUser, "shared history")}}
	plan := &agentplan.Plan{ID: "plan", Steps: []agentplan.Step{{ID: "a", Title: "first"}, {ID: "b", Title: "second"}}}
	svc := ExecutionService{Messages: appendingMessages{current}, Generation: isolatedStepGenerator{}}
	results := svc.executeReadyBatch(context.Background(), current, plan, []int{0, 1}, "run", modelport.ChatStreamHandler{}, false)
	for _, result := range results {
		if result.Err != nil {
			t.Fatal(result.Err)
		}
	}
	if len(current.Messages) != 3 {
		t.Fatalf("shared history lost step inputs: %d", len(current.Messages))
	}
}
