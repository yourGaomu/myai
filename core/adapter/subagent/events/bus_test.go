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

func TestBusClosesSubscriberWhenReplayCursorFallsOutsideWindow(t *testing.T) {
	bus := NewBus()
	bus.historyLimit = 2
	for index := 0; index < 4; index++ {
		bus.TaskUpdated(context.Background(), domainsubagent.Task{ID: "task-1", Status: domainsubagent.TaskStatusRunning})
	}
	events, cancel := bus.SubscribeTaskEvents("", 1, 1)
	defer cancel()
	if _, ok := <-events; ok {
		t.Fatal("expected stale replay cursor to close the stream")
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

func TestBusesSharingRepositoryUseOneEventSequence(t *testing.T) {
	repository := memory.NewRepository()
	first := NewBus(repository)
	second := NewBus(repository)
	first.TaskUpdated(context.Background(), domainsubagent.Task{ID: "task-1", Status: domainsubagent.TaskStatusRunning})
	second.TaskUpdated(context.Background(), domainsubagent.Task{ID: "task-2", Status: domainsubagent.TaskStatusRunning})
	first.TaskUpdated(context.Background(), domainsubagent.Task{ID: "task-3", Status: domainsubagent.TaskStatusRunning})

	events, err := repository.ListTaskEvents(context.Background(), "", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].Sequence != 1 || events[1].Sequence != 2 || events[2].Sequence != 3 {
		t.Fatalf("expected shared sequences 1,2,3, got %#v", events)
	}
}

func TestBusReplaysPersistedEventsWhenMemoryWindowExpired(t *testing.T) {
	repository := memory.NewRepository()
	first := NewBus(repository)
	for index := 0; index < 520; index++ {
		first.TaskUpdated(context.Background(), domainsubagent.Task{
			ID: "task-1", ParentSessionID: "parent-1", Status: domainsubagent.TaskStatusRunning,
		})
	}

	// NewBus keeps only the latest in-memory window. The durable repository
	// must fill the gap when a reconnecting cursor predates that window.
	second := NewBus(repository)
	events, cancel := second.SubscribeTaskEvents("parent-1", 1, 1)
	defer cancel()
	for expected := uint64(2); expected <= 520; expected++ {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatalf("event stream closed at sequence %d", expected)
			}
			if event.Sequence != expected {
				t.Fatalf("expected sequence %d, got %d", expected, event.Sequence)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for sequence %d", expected)
		}
	}
}
