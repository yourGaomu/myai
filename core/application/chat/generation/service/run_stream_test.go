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

func (ids *runStreamIDs) NewID() string {
	ids.next++
	return fmt.Sprintf("run-stream-%d", ids.next)
}
