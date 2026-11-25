package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"myai/core/adapter/chat/pendinginput"
	toolcatalog "myai/core/adapter/tool/catalog"
	toolexecutor "myai/core/adapter/tool/executor"
	generationcommand "myai/core/application/chat/generation/command"
	generationresult "myai/core/application/chat/generation/result"
	"myai/core/contextmgr"
	domaininteraction "myai/core/domain/interaction"
	domainmessage "myai/core/domain/message"
	domainsubagent "myai/core/domain/subagent"
	modelport "myai/core/port/model"
	"myai/core/session"
	"myai/core/tool"
	"myai/core/tool/local"
)

type pendingContextProvider struct{}

func (pendingContextProvider) Snapshot(current *session.Session) contextmgr.Snapshot {
	return contextmgr.BuildSnapshot(current.Messages, current.Summary, current.CompactedMessages, current.ContextWindowK)
}

type pendingModel struct {
	responses []modelport.ChatResult
	errors    []error
	calls     int
	requests  []modelport.GenerateRequest
}

func (model *pendingModel) Generate(_ context.Context, request modelport.GenerateRequest) (modelport.ChatResult, error) {
	model.requests = append(model.requests, request)
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

func TestAgentLoopFailsClosedOnConflictingSourceID(t *testing.T) {
	queue := pendinginput.NewQueue()
	if err := queue.EnqueueIdentified("session-1", "message-1", "new payload"); err != nil {
		t.Fatal(err)
	}
	current := &session.Session{ID: "session-1", Messages: []domainmessage.Message{{
		Role: domainmessage.RoleUser, Parts: []domainmessage.Part{{Text: "old payload"}}, SourceID: "message-1",
	}}}
	service := AgentLoopService{Contexts: pendingContextProvider{}, PendingInput: queue}
	_, err := service.Run(context.Background(), generationcommand.Run{
		Model: &pendingModel{responses: []modelport.ChatResult{{Content: "first"}, {Content: "answer"}}}, Session: current,
	})
	if err == nil {
		t.Fatal("expected source conflict to fail the turn")
	}
	items := queue.DrainIdentified("session-1")
	if len(items) != 1 || items[0].ID != "message-1" || items[0].Content != "new payload" {
		t.Fatalf("conflicting message was not retained for retry: %#v", items)
	}
}

func TestAgentLoopQueueMailboxWaitsForAnswerBoundary(t *testing.T) {
	queue := pendinginput.NewQueue()
	if err := queue.EnqueueAgentMessage("session-1", domainsubagent.AgentMessage{
		ID: "queued-message", RecipientAgentID: "parent-agent", Kind: domainsubagent.AgentMessageKindTaskResult,
		Content: "queued result", Trigger: domainsubagent.AgentMessageTriggerQueue,
		Status: domainsubagent.AgentMessagePending, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	call := domainmessage.ToolCall{ID: "call-1", Type: "function", Name: "read_file", Arguments: `{}`}
	model := &pendingModel{responses: []modelport.ChatResult{
		{ToolCalls: []domainmessage.ToolCall{call}},
		{Content: "first answer"},
		{Content: "answer after mailbox"},
	}}
	executor := &pendingToolExecutor{}
	service := AgentLoopService{Contexts: pendingContextProvider{}, PendingInput: queue, ToolExecutor: executor}
	result, err := service.Run(context.Background(), generationcommand.Run{
		Model: model, Session: &session.Session{ID: "session-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "answer after mailbox" || len(model.requests) != 3 {
		t.Fatalf("expected mailbox follow-up after answer boundary, result=%q requests=%d", result.Content, len(model.requests))
	}
	if requestContainsText(model.requests[1], "queued result") {
		t.Fatal("queue-only mailbox message was injected before the answer boundary")
	}
	if !requestContainsText(model.requests[2], "queued result") {
		t.Fatal("queue-only mailbox message was not injected on the next model turn")
	}
	if !requestContainsText(model.requests[2], "first answer") {
		t.Fatal("answer before queued mailbox input was lost")
	}
}

func TestAgentLoopSteerMailboxReopensCurrentTurn(t *testing.T) {
	queue := pendinginput.NewQueue()
	if err := queue.EnqueueAgentMessage("session-1", domainsubagent.AgentMessage{
		ID: "steer-message", RecipientAgentID: "parent-agent", Kind: domainsubagent.AgentMessageKindTaskResult,
		Content: "steered result", Trigger: domainsubagent.AgentMessageTriggerSteer,
		Status: domainsubagent.AgentMessagePending, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	call := domainmessage.ToolCall{ID: "call-1", Type: "function", Name: "read_file", Arguments: `{}`}
	model := &pendingModel{responses: []modelport.ChatResult{
		{ToolCalls: []domainmessage.ToolCall{call}},
		{Content: "answer with steer"},
	}}
	service := AgentLoopService{Contexts: pendingContextProvider{}, PendingInput: queue, ToolExecutor: &pendingToolExecutor{}}
	result, err := service.Run(context.Background(), generationcommand.Run{
		Model: model, Session: &session.Session{ID: "session-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "answer with steer" || len(model.requests) != 2 {
		t.Fatalf("expected steered follow-up in current turn, result=%q requests=%d", result.Content, len(model.requests))
	}
	if !requestContainsText(model.requests[1], "steered result") {
		t.Fatal("steer mailbox message was not injected before the next model sample")
	}
}

func requestContainsText(request modelport.GenerateRequest, expected string) bool {
	for _, message := range request.Messages {
		if strings.Contains(message.Text(), expected) {
			return true
		}
	}
	return false
}

type pendingToolExecutor struct{}

func (pendingToolExecutor) Execute(_ context.Context, command generationcommand.ToolExecution) (generationresult.ToolExecution, error) {
	messages := make([]domainmessage.Message, 0, len(command.Calls))
	for _, call := range command.Calls {
		messages = append(messages, domainmessage.ToolResultMessage(domainmessage.ToolResult{
			ToolCallID: call.ID, Name: call.Name, Content: "tool result",
		}))
	}
	return generationresult.ToolExecution{Messages: messages}, nil
}

type pendingTestAcknowledger struct {
	ids []string
}

func TestWaitBoundaryConsumesMailboxBeforeNextSample(t *testing.T) {
	queue := pendinginput.NewQueue()
	ack := &pendingTestAcknowledger{}
	queue.SetAcknowledger(ack)
	if err := queue.EnqueueIdentified("parent", "completion", "verified child result"); err != nil {
		t.Fatal(err)
	}
	model := &pendingModel{responses: []modelport.ChatResult{
		{ToolCalls: []domainmessage.ToolCall{{ID: "wait", Name: "wait_agent", Type: "function", Arguments: `{"targets":["child"]}`}}},
		{Content: "summary"},
	}}
	s := AgentLoopService{Contexts: pendingContextProvider{}, PendingInput: queue, ToolExecutor: pendingToolExecutor{}}
	_, err := s.Run(context.Background(), generationcommand.Run{Model: model, Session: &session.Session{ID: "parent"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 2 || !requestContainsText(model.requests[1], "verified child result") {
		t.Fatal("wait returned without exposing mailbox to the next model sample")
	}
	if len(ack.ids) != 1 {
		t.Fatalf("completion acknowledgements=%v", ack.ids)
	}
}

func TestWaitRoundsDoNotExhaustWorkspaceToolBudget(t *testing.T) {
	model := &pendingModel{responses: []modelport.ChatResult{
		{ToolCalls: []domainmessage.ToolCall{{ID: "w1", Name: "wait_agent", Type: "function", Arguments: `{}`}}},
		{ToolCalls: []domainmessage.ToolCall{{ID: "w2", Name: "wait_agent", Type: "function", Arguments: `{}`}}},
		{ToolCalls: []domainmessage.ToolCall{{ID: "work", Name: "read_file", Type: "function", Arguments: `{}`}}},
		{Content: "done"},
	}}
	s := AgentLoopService{Contexts: pendingContextProvider{}, ToolExecutor: pendingToolExecutor{}}
	current := &session.Session{ID: "parent", MaxToolRounds: 1}
	_, err := s.Run(context.Background(), generationcommand.Run{Model: model, Session: current})
	if err != nil {
		t.Fatal(err)
	}
	if model.calls != 4 {
		t.Fatalf("waits consumed work budget: %d samples", model.calls)
	}
	found := false
	for _, msg := range current.Messages {
		for _, part := range msg.Parts {
			if part.ToolCall != nil && part.ToolCall.Name == "read_file" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("workspace tool was not executed after waits")
	}
}

func TestRepeatedWaitsAreBoundedWithoutFalseFinalAnswer(t *testing.T) {
	model := &pendingModel{}
	for i := 0; i < 40; i++ {
		model.responses = append(model.responses, modelport.ChatResult{ToolCalls: []domainmessage.ToolCall{{ID: fmt.Sprintf("wait-%d", i), Name: "wait_agent", Arguments: `{}`}}})
	}
	s := AgentLoopService{Contexts: pendingContextProvider{}, ToolExecutor: pendingToolExecutor{}}
	_, err := s.Run(context.Background(), generationcommand.Run{Model: model, Session: &session.Session{ID: "parent", MaxToolRounds: 1}})
	if err == nil || !strings.Contains(err.Error(), "wait limit") || model.calls != 32 {
		t.Fatalf("wait budget: calls=%d err=%v", model.calls, err)
	}
}

func (acknowledger *pendingTestAcknowledger) Acknowledge(messageID string) error {
	acknowledger.ids = append(acknowledger.ids, messageID)
	return nil
}

func TestAgentLoopResumesAfterStructuredClarification(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("Project overview"), 0o600); err != nil {
		t.Fatal(err)
	}
	tools := tool.NewRegisterTools()
	tools.Register(local.NewReadFileToolWithWorkspace(workspace))
	tools.Register(local.NewAskUserTool())
	for _, tt := range []struct {
		name   string
		answer domaininteraction.Answer
		result string
	}{
		{
			name: "answered",
			answer: domaininteraction.Answer{Structured: map[string]domaininteraction.QuestionAnswer{
				"style": {Selected: []string{"plain"}},
			}},
			result: `{"structured_answers":{"style":{"selected":["plain"]}}}`,
		},
		{name: "skipped", answer: domaininteraction.Answer{Cancelled: true}, result: `{"skipped":true}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			model := &pendingModel{responses: []modelport.ChatResult{
				{ToolCalls: []domainmessage.ToolCall{{ID: "inspect", Name: "read_file", Type: "function", Arguments: `{"path":"README.md"}`}}},
				{ToolCalls: []domainmessage.ToolCall{{ID: "clarify", Name: "ask_user", Type: "function", Arguments: `{"decision_impact":"high","decision_reason":"The visual direction changes implementation.","context_evidence":[{"source":"workspace","path":"README.md","tool":"read_file","finding":"The project overview lacks branding."}],"title":"Confirm direction","questions":[{"id":"style","prompt":"Which style?","multi_select":false,"choices":[{"label":"plain"}]}]}`}}},
				{Content: "Continue with the available information"},
			}}
			s := AgentLoopService{Contexts: pendingContextProvider{}, Tools: toolcatalog.Catalog{Tools: tools}, ToolExecutor: toolexecutor.Executor{Registry: tools}}
			asks := 0
			response, err := s.Run(context.Background(), generationcommand.Run{
				Model: model, Session: &session.Session{ID: "session", WorkspaceRoot: workspace},
				Stream: modelport.ChatStreamHandler{OnUserQuestion: func(_ context.Context, request domaininteraction.Request) (domaininteraction.Answer, error) {
					asks++
					if request.ToolCallID != "clarify" || len(request.Questions) != 1 {
						t.Fatalf("unexpected question: %#v", request)
					}
					return tt.answer, nil
				}},
			})
			if err != nil || asks != 1 || model.calls != 3 || response.Content != "Continue with the available information" {
				t.Fatalf("run did not resume: response=%#v errors=%v asks=%d samples=%d", response, err, asks, model.calls)
			}
			for _, message := range model.requests[2].Messages {
				if result, ok := message.FirstToolResult(); ok && result.ToolCallID == "clarify" && result.Content == tt.result {
					return
				}
			}
			t.Fatal("resumed model sample did not receive the clarification result")
		})
	}
}
