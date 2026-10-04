package local

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	memoryretrievalcommand "myai/core/application/memory/retrieval/command"
	memoryretrievalresult "myai/core/application/memory/retrieval/result"
	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

func TestMemorySearchToolDefinition(t *testing.T) {
	searchTool := NewMemorySearchTool(&fakeMemoryContextPreparer{})
	if searchTool.Name() != "memory_search" {
		t.Fatalf("Name() = %q", searchTool.Name())
	}
	if searchTool.Permission() != tooldef.PermissionRead {
		t.Fatalf("Permission() = %q, want %q", searchTool.Permission(), tooldef.PermissionRead)
	}
	schema, ok := searchTool.Schema().(map[string]any)
	if !ok || schema["additionalProperties"] != false {
		t.Fatalf("schema must reject additional properties: %#v", searchTool.Schema())
	}
	if required, ok := schema["required"].([]string); !ok || !reflect.DeepEqual(required, []string{"query"}) {
		t.Fatalf("unexpected required fields: %#v", schema["required"])
	}
}

func TestMemorySearchToolUsesCurrentSessionAndReturnsMatches(t *testing.T) {
	retrieval := &fakeMemoryContextPreparer{result: memoryretrievalresult.Context{
		Triggered: true, Query: "retry extraction", MemoryIDs: []string{"memory-1"}, Prompt: "Past retry lesson",
	}}
	searchTool := NewMemorySearchTool(retrieval)
	ctx := toolruntime.WithExecution(context.Background(), toolruntime.Execution{
		SessionID: "session-1", WorkspaceRoot: `D:\project`,
	})

	output, err := searchTool.Call(ctx, json.RawMessage(`{"query":"  retry extraction  "}`))
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if retrieval.calls != 1 || retrieval.command != (memoryretrievalcommand.Prepare{Input: "retry extraction", SessionID: "session-1"}) {
		t.Fatalf("unexpected retrieval command: calls=%d command=%#v", retrieval.calls, retrieval.command)
	}
	var result struct {
		Query     string   `json:"query"`
		Found     bool     `json:"found"`
		MemoryIDs []string `json:"memory_ids"`
		Memories  string   `json:"memories"`
	}
	if err := json.Unmarshal([]byte(output.Content), &result); err != nil {
		t.Fatalf("decode tool output: %v", err)
	}
	if result.Query != "retry extraction" || !result.Found || !reflect.DeepEqual(result.MemoryIDs, []string{"memory-1"}) || result.Memories != "Past retry lesson" {
		t.Fatalf("unexpected tool result: %#v", result)
	}
}

func TestMemorySearchToolRequiresRuntimeSession(t *testing.T) {
	retrieval := &fakeMemoryContextPreparer{}
	_, err := NewMemorySearchTool(retrieval).Call(context.Background(), json.RawMessage(`{"query":"previous decision"}`))
	if err == nil {
		t.Fatal("Call() error = nil, want missing session error")
	}
	if retrieval.calls != 0 {
		t.Fatalf("retrieval calls = %d, want 0", retrieval.calls)
	}
}

func TestMemorySearchToolEnforcesPerTurnBudget(t *testing.T) {
	retrieval := &fakeMemoryContextPreparer{result: memoryretrievalresult.Context{Query: "past decision"}}
	searchTool := NewMemorySearchTool(retrieval)
	ctx := toolruntime.WithExecution(context.Background(), toolruntime.Execution{SessionID: "session-1"})
	ctx = toolruntime.WithMemorySearchBudget(ctx, toolruntime.NewMemorySearchBudget(1))
	args := json.RawMessage(`{"query":"past decision"}`)
	if _, err := searchTool.Call(ctx, args); err != nil {
		t.Fatalf("first Call() error = %v", err)
	}
	if _, err := searchTool.Call(ctx, args); err == nil {
		t.Fatal("second Call() error = nil, want budget error")
	}
	if retrieval.calls != 1 {
		t.Fatalf("retrieval calls = %d, want 1", retrieval.calls)
	}
}

func TestMemorySearchToolReturnsNoMatchResult(t *testing.T) {
	retrieval := &fakeMemoryContextPreparer{result: memoryretrievalresult.Context{Query: "missing memory"}}
	ctx := toolruntime.WithExecution(context.Background(), toolruntime.Execution{SessionID: "session-1"})
	output, err := NewMemorySearchTool(retrieval).Call(ctx, json.RawMessage(`{"query":"missing memory"}`))
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	var result struct {
		Found bool `json:"found"`
	}
	if err := json.Unmarshal([]byte(output.Content), &result); err != nil {
		t.Fatalf("decode tool output: %v", err)
	}
	if result.Found {
		t.Fatalf("Found = true, want false: %s", output.Content)
	}
}

type fakeMemoryContextPreparer struct {
	command memoryretrievalcommand.Prepare
	result  memoryretrievalresult.Context
	err     error
	calls   int
}

func (preparer *fakeMemoryContextPreparer) Prepare(_ context.Context, command memoryretrievalcommand.Prepare) (memoryretrievalresult.Context, error) {
	preparer.calls++
	preparer.command = command
	if preparer.err != nil {
		return memoryretrievalresult.Context{}, preparer.err
	}
	return preparer.result, nil
}
