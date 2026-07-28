package events

import (
	"context"
	"testing"

	domainsubagent "myai/core/domain/subagent"
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
