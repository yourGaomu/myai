package sessionapp

import (
	"context"
	"testing"

	memorysession "myai/core/adapter/session/memory"
	"myai/core/llm"
	"myai/core/session"
)

func TestMessageCommandServiceAppendUserMessage(t *testing.T) {
	memory := memorysession.NewStore("gpt-5")
	if err := memory.PutSessionWithOptions("session-1", "gpt-5", session.PermissionModeAsk, 0, nil); err != nil {
		t.Fatal(err)
	}
	service := newMessageCommandService(memory)

	result, err := service.AppendUserMessage(context.Background(), AppendUserMessageCommand{
		Input: "  hello  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Session.ID != "session-1" || result.Input != "  hello  " {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(result.Session.Messages) != 2 || result.Session.Messages[1].Text() != "  hello  " {
		t.Fatalf("expected user message to be appended without changing its content: %#v", result.Session.Messages)
	}
}

func TestMessageCommandServiceRejectsEmptyUserMessage(t *testing.T) {
	memory := memorysession.NewStore("gpt-5")
	service := newMessageCommandService(memory)

	if _, err := service.AppendUserMessage(context.Background(), AppendUserMessageCommand{Input: "   "}); err == nil {
		t.Fatal("expected empty input error")
	}
}

func TestMessageCommandServicePrepareRegeneration(t *testing.T) {
	memory := memorysession.NewStore("gpt-5")
	if err := memory.PutSessionWithOptions("session-1", "gpt-5", session.PermissionModeAsk, 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := memory.AddUserMessageTo("session-1", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := memory.AddAssistantMessageTo("session-1", "first answer"); err != nil {
		t.Fatal(err)
	}
	if err := memory.AddUsageTo("session-1", llm.TokenUsage{TotalTokens: 12}); err != nil {
		t.Fatal(err)
	}

	result, err := newMessageCommandService(memory).PrepareRegeneration(context.Background(), PrepareRegenerationCommand{
		SessionID: " session-1 ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Input != "hello" {
		t.Fatalf("expected last user input, got %q", result.Input)
	}
	if len(result.Session.Messages) != 2 || result.Session.Messages[1].Text() != "hello" {
		t.Fatalf("expected assistant response to be trimmed: %#v", result.Session.Messages)
	}
	if result.Session.LastUsage != (llm.TokenUsage{}) {
		t.Fatalf("expected last usage to be reset: %#v", result.Session.LastUsage)
	}
}

func newMessageCommandService(memory *memorysession.Store) MessageCommandService {
	return MessageCommandService{
		Loader: LoadService{Memory: memory},
		Memory: memory,
	}
}
