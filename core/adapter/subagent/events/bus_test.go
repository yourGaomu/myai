package events

import (
	"context"
	"testing"
	"time"

	"myai/core/adapter/subagent/memory"
	domainsubagent "myai/core/domain/subagent"
	subagentport "myai/core/port/subagent"
)

func TestBusKeepsLatestTaskUpdateWhenSubscriberIsSlow(t *testing.T) {
	bus := NewBus()
	events, unsubscribe := bus.Subscribe(1)
	defer unsubscribe()

	bus.TaskUpdated(context.Background(), domainsubagent.Task{ID: "task-1", Status: domainsubagent.TaskStatusRunning})
	bus.TaskUpdated(context.Background(), domainsubagent.Task{ID: "task-1", Status: domainsubagent.TaskStatusSucceeded})

	event := <-events
	if event.Status != domainsubagent.TaskStatusSucceeded {
		t.Fatalf("expected latest terminal update, got %s", event.Status)
	}
}

func TestBusReplaysOrderedParentTaskEvents(t *testing.T) {
	bus := NewBus()
	parent := "parent-1"
	bus.TaskUpdated(context.Background(), domainsubagent.Task{ID: "task-1", ParentSessionID: parent, Status: domainsubagent.TaskStatusQueued})
	bus.TaskUpdated(context.Background(), domainsubagent.Task{ID: "task-1", ParentSessionID: parent, Status: domainsubagent.TaskStatusRunning})
	bus.TaskUpdated(context.Background(), domainsubagent.Task{ID: "task-1", ParentSessionID: parent, Status: domainsubagent.TaskStatusSucceeded})

	events, unsubscribe := bus.SubscribeTaskEvents(parent, 1, 4)
	defer unsubscribe()
	first, ok := <-events
	if !ok || first.Sequence != 2 || first.Kind != TaskEventKindUpdated {
		t.Fatalf("unexpected replayed first event: %#v", first)
	}
	second, ok := <-events
	if !ok || second.Sequence != 3 || second.Kind != TaskEventKindCompleted {
		t.Fatalf("unexpected replayed terminal event: %#v", second)
	}
}

func TestBusClosesSlowEventSubscriberInsteadOfDroppingSequence(t *testing.T) {
	bus := NewBus()
	events, _ := bus.SubscribeTaskEvents("parent-1", 0, 1)
	for index := 0; index < 3; index++ {
		bus.TaskUpdated(context.Background(), domainsubagent.Task{
			ID: "task-1", ParentSessionID: "parent-1", Status: domainsubagent.TaskStatusRunning,
		})
	}
	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("expected slow event subscriber to be closed")
		}
	}
}

func TestBusRestoresPersistedSequenceAndHistory(t *testing.T) {
	repository := memory.NewRepository()
	first := NewBus(repository)
	first.TaskUpdated(context.Background(), domainsubagent.Task{ID: "task-1", ParentSessionID: "parent-1", Status: domainsubagent.TaskStatusRunning})
	first.PublishTaskEvent(context.Background(), subagentport.TaskEvent{
		Kind: subagentport.TaskEventKindReasoning,
		Task: domainsubagent.Task{ID: "task-1", ParentSessionID: "parent-1"}, Content: "inspect", Delta: true,
	})

	second := NewBus(repository)
	events, cancel := second.SubscribeTaskEvents("parent-1", 1, 1)
	defer cancel()
	select {
	case event := <-events:
		if event.Sequence != 2 || event.Kind != subagentport.TaskEventKindReasoning || event.Content != "inspect" {
			t.Fatalf("unexpected restored event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for restored event")
	}
	second.TaskUpdated(context.Background(), domainsubagent.Task{ID: "task-2", ParentSessionID: "parent-1", Status: domainsubagent.TaskStatusSucceeded})
	newEvents, newCancel := second.SubscribeTaskEvents("parent-1", 2, 1)
	defer newCancel()
	select {
	case event := <-newEvents:
		if event.Sequence != 3 {
			t.Fatalf("expected sequence to continue after restore, got %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for post-restore event")
	}
}
