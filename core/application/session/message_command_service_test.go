package sessionapp

import (
	"context"
	"errors"
	"testing"

	memorysession "myai/core/adapter/session/memory"
	domainmessage "myai/core/domain/message"
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

func TestMessageCommandServiceAppendsRuntimeInstructionBeforeUser(t *testing.T) {
	memory := memorysession.NewStore("gpt-5")
	if err := memory.PutSessionWithOptions("session-1", "gpt-5", session.PermissionModeAsk, 0, nil); err != nil {
		t.Fatal(err)
	}
	provider := &messageRuntimeProvider{prompt: "plan rules"}
	service := newMessageCommandService(memory)
	service.RuntimeInstructions = provider

	result, err := service.AppendUserMessage(context.Background(), AppendUserMessageCommand{
		SessionID: "session-1", Input: "write a poem", ForceChatMode: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RuntimeInstruction != "plan rules" || provider.input != "write a poem" || !provider.forceChatMode {
		t.Fatalf("unexpected runtime instruction result: result=%#v provider=%#v", result, provider)
	}
	if len(result.Session.Messages) != 3 {
		t.Fatalf("expected system, runtime, and user messages: %#v", result.Session.Messages)
	}
	if !result.Session.Messages[1].IsSyntheticReason(domainmessage.SyntheticReasonRuntimeInstruction) {
		t.Fatalf("expected persisted runtime message before user: %#v", result.Session.Messages[1])
	}
	if result.Session.Messages[2].Role != domainmessage.RoleUser || result.Session.Messages[2].Text() != "write a poem" {
		t.Fatalf("unexpected user message: %#v", result.Session.Messages[2])
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

func TestMessageCommandServiceRestoresMemoryWhenRegenerationPersistenceFails(t *testing.T) {
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

	expected := errors.New("transaction failed")
	service := newMessageCommandService(memory)
	service.Regeneration = failingRegenerationPersistence{err: expected}
	if _, err := service.PrepareRegeneration(context.Background(), PrepareRegenerationCommand{SessionID: "session-1"}); !errors.Is(err, expected) {
		t.Fatalf("expected persistence failure, got %v", err)
	}

	current, err := memory.GetSession("session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Messages) != 3 || current.Messages[2].Text() != "first answer" {
		t.Fatalf("expected original transcript to be restored: %#v", current.Messages)
	}
	if current.LastUsage.TotalTokens != 12 {
		t.Fatalf("expected last usage to be restored: %#v", current.LastUsage)
	}
}

func newMessageCommandService(memory *memorysession.Store) MessageCommandService {
	return MessageCommandService{
		Loader: LoadService{Memory: memory},
		Memory: memory,
	}
}

type messageRuntimeProvider struct {
	prompt        string
	input         string
	forceChatMode bool
}

type failingRegenerationPersistence struct {
	err error
}

func (p failingRegenerationPersistence) PersistRegeneratedSession(context.Context, *session.Session) error {
	return p.err
}

func (p *messageRuntimeProvider) Prompt(_ context.Context, _ *session.Session, input string, forceChatMode bool) string {
	p.input = input
	p.forceChatMode = forceChatMode
	return p.prompt
}
