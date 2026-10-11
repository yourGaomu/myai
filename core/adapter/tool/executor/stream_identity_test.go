package executor

import (
	"context"
	"sync"
	"testing"

	generationcommand "myai/core/application/chat/generation/command"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	modelport "myai/core/port/model"
	"myai/core/session"
	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

func TestConcurrentSameNameToolsKeepIndependentCallIdentity(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	started := map[string]int{}
	finished := map[string]int{}
	tool := fakeTool{name: "inspect_execution", permission: tooldef.PermissionRead, inspect: func(ctx context.Context) {
		execution, _ := toolruntime.CurrentExecution(ctx)
		mu.Lock()
		defer mu.Unlock()
		seen[execution.ToolCallID]++
	}}
	executor := Executor{Registry: fakeRegistry{tools: map[string]tooldef.Tool{"inspect_execution": tool}}}
	_, err := executor.Execute(context.Background(), generationcommand.ToolExecution{
		Session: &session.Session{ID: "parent", PermissionMode: session.PermissionModeReadonly},
		Calls:   []domainmessage.ToolCall{{ID: "call-a", Name: "inspect_execution"}, {ID: "call-b", Name: "inspect_execution"}},
		Stream: modelport.ChatStreamHandler{
			OnToolCall:      func(string, string) { t.Error("legacy callback must not duplicate identified event") },
			OnToolCallEvent: func(event modelport.ToolCallEvent) { mu.Lock(); defer mu.Unlock(); started[event.ToolCallID]++ },
			OnToolResult:    func(event modelport.ToolResultEvent) { mu.Lock(); defer mu.Unlock(); finished[event.ToolCallID]++ },
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"call-a", "call-b"} {
		if seen[id] != 1 || started[id] != 1 || finished[id] != 1 {
			t.Fatalf("mismatched identity: seen=%v started=%v finished=%v", seen, started, finished)
		}
	}
}

func TestSharedAssetEventUsesSameIDAsPersistedBatch(t *testing.T) {
	var streamed domaintool.SharedAsset
	executor := Executor{Registry: fakeRegistry{tools: map[string]tooldef.Tool{
		"share_file": fakeTool{name: "share_file", permission: tooldef.PermissionRead, result: `{"file_name":"结果.png","content_type":"image/png","short_url":"https://example.com/a","code":"a"}`},
	}}}
	result, err := executor.Execute(context.Background(), generationcommand.ToolExecution{
		Session: &session.Session{ID: "child", PermissionMode: session.PermissionModeReadonly},
		Calls:   []domainmessage.ToolCall{{ID: "image-call", Name: "share_file"}},
		Stream:  modelport.ChatStreamHandler{OnAsset: func(asset domaintool.SharedAsset) { streamed = asset }},
	})
	if err != nil || len(result.Assets) != 1 {
		t.Fatalf("asset result: %#v %v", result, err)
	}
	if streamed.ID == "" || streamed.ID != result.Assets[0].ID || streamed.ToolCallID != "image-call" || streamed.FileName != "结果.png" {
		t.Fatalf("asset identity lost: %#v %#v", streamed, result.Assets)
	}
}
