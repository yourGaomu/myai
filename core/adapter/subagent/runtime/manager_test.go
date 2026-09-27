package runtime

import (
	"errors"
	"testing"

	domainsubagent "myai/core/domain/subagent"
	subagentport "myai/core/port/subagent"
)

func TestManagerReservesOnlyOneTurnPerThread(t *testing.T) {
	manager := New()
	thread := domainsubagent.AgentThread{
		ID: "thread-1", ParentSessionID: "root", AgentPath: "/root/research",
		Status: domainsubagent.TaskStatusQueued,
	}
	if err := manager.Register(thread); err != nil {
		t.Fatal(err)
	}
	if err := manager.ReserveTurn(thread.ID, "turn-1"); err != nil {
		t.Fatal(err)
	}
	if err := manager.ReserveTurn(thread.ID, "turn-2"); !errors.Is(err, subagentport.ErrAgentTurnActive) {
		t.Fatalf("expected active-turn conflict, got %v", err)
	}
	if err := manager.ReleaseTurn(thread.ID, "turn-2"); !errors.Is(err, subagentport.ErrAgentTurnMismatch) {
		t.Fatalf("expected stale turn rejection, got %v", err)
	}
	if err := manager.ReleaseTurn(thread.ID, "turn-1"); err != nil {
		t.Fatal(err)
	}
	if err := manager.ReserveTurn(thread.ID, "turn-2"); err != nil {
		t.Fatal(err)
	}
}

func TestManagerSupportsLoadedAndUnloadedResidency(t *testing.T) {
	manager := New()
	thread := domainsubagent.AgentThread{ID: "thread-1", Status: domainsubagent.TaskStatusSucceeded}
	if err := manager.Register(thread); err != nil {
		t.Fatal(err)
	}
	if err := manager.MarkUnloaded(thread.ID); !errors.Is(err, subagentport.ErrAgentRuntimeUnloaded) {
		t.Fatalf("new runtime should already be unloaded, got %v", err)
	}
	if err := manager.MarkLoaded(thread.ID); err != nil {
		t.Fatal(err)
	}
	if err := manager.MarkLoaded(thread.ID); !errors.Is(err, subagentport.ErrAgentRuntimeLoaded) {
		t.Fatalf("expected duplicate load rejection, got %v", err)
	}
	if err := manager.MarkUnloaded(thread.ID); err != nil {
		t.Fatal(err)
	}
	snapshot, err := manager.Inspect(thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Loaded || snapshot.Status != domainsubagent.AgentStatusUnloaded {
		t.Fatalf("unexpected unloaded snapshot: %#v", snapshot)
	}
	if err := manager.Remove(thread.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Inspect(thread.ID); !errors.Is(err, subagentport.ErrAgentRuntimeNotFound) {
		t.Fatalf("expected removed runtime to be absent, got %v", err)
	}
}

func TestManagerRegisterRefreshesMetadataWithoutDroppingResidency(t *testing.T) {
	manager := New()
	if err := manager.Register(domainsubagent.AgentThread{ID: "thread-1", AgentPath: "root/old", Status: domainsubagent.TaskStatusQueued}); err != nil {
		t.Fatal(err)
	}
	if err := manager.MarkLoaded("thread-1"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Register(domainsubagent.AgentThread{
		ID: "thread-1", ParentTaskID: "parent", ParentSessionID: "root",
		AgentPath: "/root/new", AgentNickname: "researcher", Status: domainsubagent.TaskStatusRunning,
		CurrentRunID: "turn-1",
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := manager.Inspect("thread-1")
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Loaded || snapshot.AgentPath != "root/new" || snapshot.ParentThreadID != "parent" || snapshot.Status != domainsubagent.AgentStatusRunning {
		t.Fatalf("metadata refresh lost state: %#v", snapshot)
	}
}
