package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	memoryadapter "myai/core/adapter/session/memory"
	compactionresult "myai/core/application/chat/compaction/result"
	contextservice "myai/core/application/chat/context/service"
	command "myai/core/application/chat/generation/command"
	result "myai/core/application/chat/generation/result"
	"myai/core/contextmgr"
	message "myai/core/domain/message"
	modelport "myai/core/port/model"
	"myai/core/session"
)

type overflowCompactor struct {
	calls   int
	windows []int
}

func (c *overflowCompactor) CompactIfNeeded(context.Context, *session.Session, modelport.ChatModelPort) (compactionresult.CompactInfo, error) {
	return compactionresult.CompactInfo{}, nil
}
func (c *overflowCompactor) CompactIfNeededForRequest(_ context.Context, current *session.Session, _ modelport.ChatModelPort, options contextmgr.RequestOptions) (compactionresult.CompactInfo, error) {
	c.calls++
	c.windows = append(c.windows, options.Budget.WindowTokens)
	if options.Budget.WindowTokens < 16000 {
		// This fake models a persisted checkpoint being projected by a provider.
		current.Messages = []message.Message{message.Text(message.RoleSystem, "system"), message.Text(message.RoleUser, "finish")}
	}
	return compactionresult.CompactInfo{}, nil
}

type recoveryTools struct{}

func (recoveryTools) ToolsForSession(*session.Session, bool) []modelport.Tool {
	return []modelport.Tool{{Type: "function", Function: &modelport.FunctionDefinition{Name: "read_file", Parameters: map[string]any{"type": "object"}}}}
}

type recoveryExecutor struct{ calls int }

func (e *recoveryExecutor) Execute(_ context.Context, c command.ToolExecution) (result.ToolExecution, error) {
	e.calls++
	return result.ToolExecution{Calls: c.Calls, Messages: []message.Message{message.ToolResultMessage(message.ToolResult{ToolCallID: c.Calls[0].ID, Name: "read_file", Content: "done"})}}, nil
}

func TestContextOverflowCompactsOnceWithoutReplayingExecutedTool(t *testing.T) {
	for _, repeated := range []bool{false, true} {
		t.Run(fmt.Sprint(repeated), func(t *testing.T) {
			model := &pendingModel{responses: []modelport.ChatResult{
				{ToolCalls: []message.ToolCall{{ID: "read-1", Name: "read_file", Arguments: `{}`}}}, {}, {Content: "finished"},
			}, errors: []error{nil, modelport.ErrContextWindowExceeded, nil}}
			if repeated {
				model.errors[2] = modelport.ErrContextWindowExceeded
			}
			compactor := &overflowCompactor{}
			executor := &recoveryExecutor{}
			current := &session.Session{ID: "session", Messages: []message.Message{message.Text(message.RoleSystem, "system"), message.Text(message.RoleUser, strings.Repeat("history ", 1000))}}
			response, err := (AgentLoopService{Contexts: contextservice.SnapshotService{}, Tools: recoveryTools{}, ToolExecutor: executor, Compactor: compactor}).Run(context.Background(), command.Run{Model: model, Session: current, ModelWindowTokens: 16000})
			if repeated {
				if !errors.Is(err, modelport.ErrContextWindowExceeded) {
					t.Fatalf("expected second overflow, got %v", err)
				}
			} else if err != nil || response.Content != "finished" {
				t.Fatalf("recovery response=%+v err=%v", response, err)
			}
			if model.calls != 3 || executor.calls != 1 {
				t.Fatalf("model calls=%d tool calls=%d", model.calls, executor.calls)
			}
			if len(compactor.windows) != 3 || compactor.windows[2] >= compactor.windows[1] {
				t.Fatalf("missing reduced-budget retry: %v", compactor.windows)
			}
		})
	}
}

func TestInvalidHistoricalToolJSONIsQuarantinedWithResultsWithoutMutatingStore(t *testing.T) {
	memory := memoryadapter.NewStore("model")
	history := []message.Message{
		message.Text(message.RoleSystem, "system"), message.Text(message.RoleUser, "old"),
		message.Assistant("old exchange", "", []message.ToolCall{{ID: "bad", Name: "read_file", Arguments: `{"path":`}, {ID: "sibling", Name: "read_file", Arguments: `{}`}}),
		message.ToolResultMessage(message.ToolResult{ToolCallID: "bad", Content: "bad old result"}),
		message.ToolResultMessage(message.ToolResult{ToolCallID: "sibling", Content: "sibling result"}),
		message.Text(message.RoleUser, "continue"),
	}
	if err := memory.PutSession("session", "model", history); err != nil {
		t.Fatal(err)
	}
	before, _ := memory.Snapshot("session")
	model := &pendingModel{responses: []modelport.ChatResult{{Content: "recovered"}}}
	_, err := (AgentLoopService{Sessions: memory, Contexts: contextservice.SnapshotService{}}).Run(context.Background(), command.Run{Model: model, Session: before})
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range model.requests[0].Messages {
		if msg.HasToolCall() || msg.Role == message.RoleTool {
			t.Fatal("quarantined exchange left orphan tool messages")
		}
	}
	if !requestContainsText(model.requests[0], "invalid JSON") {
		t.Fatal("missing recovery context")
	}
	after, _ := memory.Snapshot("session")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("request projection mutated raw session")
	}
}

type failingStopHooks struct{ calls int }

func (h *failingStopHooks) Handle(context.Context, command.TurnHook) (command.TurnHookOutcome, error) {
	h.calls++
	return command.TurnHookOutcome{}, errors.New("hook failed")
}
func TestStopHookFailurePreservesAnswerAndInternalRunSkipsStop(t *testing.T) {
	for _, internal := range []bool{false, true} {
		t.Run(fmt.Sprint(internal), func(t *testing.T) {
			hooks := &failingStopHooks{}
			model := &pendingModel{responses: []modelport.ChatResult{{Content: "completed answer"}}}
			reported := 0
			response, err := (AgentLoopService{Contexts: contextservice.SnapshotService{}, TurnHooks: hooks, OnHookError: func(error) { reported++ }}).Run(context.Background(), command.Run{Session: &session.Session{ID: "session"}, Model: model, Internal: internal})
			if err != nil || response.Content != "completed answer" {
				t.Fatalf("answer lost: %+v %v", response, err)
			}
			expected := 1
			if internal {
				expected = 0
			}
			if hooks.calls != expected || reported != expected {
				t.Fatalf("hook=%d reported=%d", hooks.calls, reported)
			}
		})
	}
}
