package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	agentrunmemory "myai/core/adapter/persistence/memory/agentrun"
	agentruncommand "myai/core/application/agentrun/command"
	agentrunservice "myai/core/application/agentrun/service"
	domainagentrun "myai/core/domain/agentrun"
	domaintool "myai/core/domain/tool"
	modelport "myai/core/port/model"
)

func TestRunStreamRecorderAggregatesReasoningAndRecordsTools(t *testing.T) {
	repository := agentrunmemory.New()
	commands := agentrunservice.CommandService{Repository: repository, IDs: &runStreamIDs{}}
	run, err := commands.Start(context.Background(), agentruncommand.Start{SessionID: "session-1"})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	var live []domainagentrun.Event
	recorder := newRunStreamRecorder(context.Background(), run.ID, commands, modelport.ChatStreamHandler{
		OnRunEvent: func(event domainagentrun.Event) { live = append(live, event) },
	}, func(err error) { t.Fatal(err) })
	handler := recorder.Handler()
	handler.OnReasoning("inspect ")
	handler.OnReasoning("workspace")
	handler.OnAnswer("answer")
	handler.OnToolCall("read_file", `{"path":"main.go"}`)
	recorder.Close()

	events, err := repository.ListEvents(context.Background(), []string{run.ID})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 || events[0].Content != "inspect workspace" || events[1].Type != domainagentrun.EventTypeToolCall {
		t.Fatalf("unexpected persisted events: %#v", events)
	}
	if len(live) != 3 || !live[1].Delta || live[1].Content != "workspace" {
		t.Fatalf("unexpected live events: %#v", live)
	}
}

func TestRunStreamRecorderBoundsReasoningBuffer(t *testing.T) {
	repository := agentrunmemory.New()
	commands := agentrunservice.CommandService{Repository: repository, IDs: &runStreamIDs{}}
	run, err := commands.Start(context.Background(), agentruncommand.Start{SessionID: "session-1"})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	recorder := newRunStreamRecorder(context.Background(), run.ID, commands, modelport.ChatStreamHandler{}, func(err error) { t.Fatal(err) })
	recorder.Handler().OnReasoning(strings.Repeat("x", maxReasoningEventBytes+32))
	recorder.Close()

	events, err := repository.ListEvents(context.Background(), []string{run.ID})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 || len(events[0].Content) != maxReasoningEventBytes || !events[0].Truncated {
		t.Fatalf("reasoning bound was not applied: %#v", events)
	}
}

type runStreamIDs struct{ next int }

func TestRunRecorderPreservesToolAndAssetIdentity(t *testing.T) {
	repository := agentrunmemory.New()
	commands := agentrunservice.CommandService{Repository: repository, IDs: &runStreamIDs{}}
	run, err := commands.Start(context.Background(), agentruncommand.Start{SessionID: "child"})
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	var assets []string
	recorder := newRunStreamRecorder(context.Background(), run.ID, commands, modelport.ChatStreamHandler{
		OnToolCallEvent: func(event modelport.ToolCallEvent) { calls = append(calls, event.ToolCallID) },
		OnAsset:         func(asset domaintool.SharedAsset) { assets = append(assets, asset.ID) },
	}, func(err error) { t.Fatal(err) })
	handler := recorder.Handler()
	handler.OnToolCallEvent(modelport.ToolCallEvent{ToolCallID: "call", Name: "share_file"})
	handler.OnToolResult(modelport.ToolResultEvent{ToolCallID: "call", Name: "share_file"})
	handler.OnAsset(domaintool.SharedAsset{ID: "image", ToolCallID: "call"})
	recorder.Close()
	events, err := repository.ListEvents(context.Background(), []string{run.ID})
	if err != nil || len(events) != 2 || events[0].ToolCallID != "call" || events[1].ToolCallID != "call" || len(calls) != 1 || calls[0] != "call" || len(assets) != 1 || assets[0] != "image" {
		t.Fatalf("identity lost: %#v %v %v %v", events, calls, assets, err)
	}
}

func TestRunStreamWaitReportsWaitingWithoutCompletingRun(t *testing.T) {
	repository := agentrunmemory.New()
	commands := agentrunservice.CommandService{Repository: repository, IDs: &runStreamIDs{}}
	run, err := commands.Start(context.Background(), agentruncommand.Start{SessionID: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	var live []domainagentrun.Event
	recorder := newRunStreamRecorder(context.Background(), run.ID, commands, modelport.ChatStreamHandler{OnRunEvent: func(e domainagentrun.Event) { live = append(live, e) }}, func(err error) { t.Fatal(err) })
	recorder.Handler().OnToolCall("wait_agent", `{"targets":["child"]}`)
	recorder.Close()
	stored, err := repository.GetRun(context.Background(), run.ID)
	if err != nil || stored.Status != domainagentrun.StatusRunning || len(live) != 1 || live[0].Status != "waiting" {
		t.Fatalf("waiting ended run: %#v %#v %v", stored, live, err)
	}
}

func (ids *runStreamIDs) NewID() string {
	ids.next++
	return fmt.Sprintf("run-stream-%d", ids.next)
}
