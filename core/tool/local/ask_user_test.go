package local

import (
	"context"
	"encoding/json"
	"testing"

	domaininteraction "myai/core/domain/interaction"
	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

func TestAskUserToolWaitsForStructuredAnswer(t *testing.T) {
	var received domaininteraction.Request
	ctx := toolruntime.WithExecution(context.Background(), toolruntime.Execution{
		SessionID: "session-1", ToolCallID: "call-1", RunID: "run-1", TaskID: "task-1",
	})
	ctx = toolruntime.WithUserQuestionHandler(ctx, func(_ context.Context, request domaininteraction.Request) (domaininteraction.Answer, error) {
		received = request
		return domaininteraction.Answer{Answers: map[string]string{"question_1": "small"}}, nil
	})
	output, err := NewAskUserTool().Call(ctx, json.RawMessage(`{"title":"Scope","questions":[{"prompt":"How much?","options":["small","large"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if received.SessionID != "session-1" || received.SourceTaskID != "task-1" || received.ToolCallID != "call-1" || received.RunID != "run-1" {
		t.Fatalf("question lost execution identity: %#v", received)
	}
	if output.Status != "success" || output.Content != `{"answers":{"question_1":"small"}}` {
		t.Fatalf("unexpected tool output: %#v", output)
	}
}

func TestAskUserToolRequiresInteractiveTransport(t *testing.T) {
	_, err := NewAskUserTool().Call(context.Background(), json.RawMessage(`{"questions":[{"prompt":"Choose"}]}`))
	if err == nil {
		t.Fatal("expected missing interaction transport to fail")
	}
	if NewAskUserTool().Permission() != tooldef.PermissionRead || !NewAskUserTool().IsTurnBarrier() {
		t.Fatal("ask_user must be a read-only turn barrier")
	}
}
