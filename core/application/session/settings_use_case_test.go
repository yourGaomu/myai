package sessionapp

import (
	"context"
	"errors"
	"testing"

	"myai/core/session"
)

func TestSettingsUseCasePersistsAndPublishesSessionChanges(t *testing.T) {
	memory := settingsMemory("session-1")
	persistence := &recordingSessionPersistence{}
	events := &recordingSessionEvents{}
	useCase := SettingsUseCase{
		Settings: SettingsService{
			Memory: memory,
			Models: fakeSettingsModelRegistry{"gpt-5": true},
		},
		Persistence: persistence,
		Events:      events,
	}
	ctx := context.Background()

	if err := useCase.SwitchModel(ctx, SwitchModelCommand{SessionID: "session-1", ModelID: "gpt-5"}); err != nil {
		t.Fatal(err)
	}
	if err := useCase.SetPermissionMode(ctx, SetPermissionModeCommand{SessionID: "session-1", Mode: string(session.PermissionModeFull)}); err != nil {
		t.Fatal(err)
	}
	if err := useCase.SetAgentMode(ctx, SetAgentModeCommand{SessionID: "session-1", Mode: string(session.AgentModePlan)}); err != nil {
		t.Fatal(err)
	}
	if err := useCase.SetContextWindow(ctx, SetContextWindowCommand{SessionID: "session-1", WindowK: 12}); err != nil {
		t.Fatal(err)
	}

	if len(persistence.commands) != 4 {
		t.Fatalf("persistence command count = %d, want 4", len(persistence.commands))
	}
	for index, command := range persistence.commands {
		if command.SessionID != "session-1" || command.Model != "gpt-5" {
			t.Fatalf("persistence command[%d] = %#v", index, command)
		}
	}
	assertSessionEvents(t, events.events,
		sessionEvent{sessionID: "session-1", reason: "model"},
		sessionEvent{sessionID: "session-1", reason: "permission"},
		sessionEvent{sessionID: "session-1", reason: "agent_mode"},
		sessionEvent{sessionID: "session-1", reason: "context"},
	)
}

func TestSettingsUseCaseDoesNotPublishWhenPersistenceFails(t *testing.T) {
	persistenceErr := errors.New("save failed")
	events := &recordingSessionEvents{}
	useCase := SettingsUseCase{
		Settings: SettingsService{
			Memory: settingsMemory("session-1"),
			Models: fakeSettingsModelRegistry{"gpt-5": true},
		},
		Persistence: &recordingSessionPersistence{err: persistenceErr},
		Events:      events,
	}

	err := useCase.SwitchModel(context.Background(), SwitchModelCommand{SessionID: "session-1", ModelID: "gpt-5"})
	if !errors.Is(err, persistenceErr) {
		t.Fatalf("SwitchModel() error = %v, want %v", err, persistenceErr)
	}
	if len(events.events) != 0 {
		t.Fatalf("unexpected events after persistence failure: %#v", events.events)
	}
}
