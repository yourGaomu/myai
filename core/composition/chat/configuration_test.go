package chat

import (
	"context"
	"testing"

	memorysession "myai/core/adapter/session/memory"
	lifecyclecommand "myai/core/application/session/lifecycle/command"
	messagecommand "myai/core/application/session/message/command"
)

func TestBuildDependenciesWiresStableSessionServices(t *testing.T) {
	memory := memorysession.NewStore("gpt-test")
	dependencies := BuildDependencies(Configuration{
		Sessions:     memory,
		DefaultModel: "gpt-test",
	})

	result, err := dependencies.SessionLifecycle.Create(context.Background(), lifecyclecommand.CreateSession{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	current := result.Current
	if current == nil || current.ID == "" {
		t.Fatalf("Create() = %#v", result)
	}
	initialMessageCount := len(current.Messages)

	prepared, err := dependencies.MessageCommands.AppendUserMessage(context.Background(), messagecommand.AppendUserMessage{
		SessionID: current.ID,
		Input:     "hello",
	})
	if err != nil {
		t.Fatalf("AppendUserMessage() error = %v", err)
	}
	if len(prepared.Session.Messages) != initialMessageCount+1 {
		t.Fatalf("message count = %d, want %d", len(prepared.Session.Messages), initialMessageCount+1)
	}

	state := dependencies.CurrentState.State()
	if state.SessionID != current.ID || state.ModelID != "gpt-test" {
		t.Fatalf("unexpected current state: %#v", state)
	}
	if dependencies.GenerationTasks == nil {
		t.Fatal("generation task service is not wired")
	}
}

func TestNewServiceUsesComposedCurrentState(t *testing.T) {
	memory := memorysession.NewStore("gpt-test")
	chatService := NewService(Configuration{
		Sessions:     memory,
		DefaultModel: "gpt-test",
	})

	if got := chatService.CurrentModelID(); got != "gpt-test" {
		t.Fatalf("CurrentModelID() = %q, want gpt-test", got)
	}
}
