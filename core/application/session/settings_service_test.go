package sessionapp

import (
	"context"
	"errors"
	"testing"

	modelport "myai/core/port/model"
	"myai/core/session"
)

func TestSettingsServiceSwitchModelForSession(t *testing.T) {
	memory := settingsMemory("session-1")

	current, err := (SettingsService{Memory: memory, Models: fakeSettingsModelRegistry{"gpt-5": true}}).SwitchModel(context.Background(), SwitchModelCommand{
		SessionID: "session-1",
		ModelID:   "gpt-5",
	})
	if err != nil {
		t.Fatal(err)
	}
	if current.Model != "gpt-5" || memory.sessions["session-1"].Model != "gpt-5" {
		t.Fatalf("expected model to be updated, got current=%#v memory=%#v", current, memory.sessions["session-1"])
	}
}

func TestSettingsServiceSwitchModelWithoutSessionUpdatesCurrentModelOnly(t *testing.T) {
	memory := settingsMemory("")

	current, err := (SettingsService{Memory: memory, Models: fakeSettingsModelRegistry{"gpt-5": true}}).SwitchModel(context.Background(), SwitchModelCommand{
		ModelID: "gpt-5",
	})
	if err != nil {
		t.Fatal(err)
	}
	if current != nil {
		t.Fatalf("expected no specific session to be returned, got %#v", current)
	}
	if memory.currentModelID != "gpt-5" {
		t.Fatalf("expected current model to be updated, got %q", memory.currentModelID)
	}
}

func TestSettingsServiceSwitchModelRejectsMissingModel(t *testing.T) {
	_, err := (SettingsService{Memory: settingsMemory("session-1"), Models: fakeSettingsModelRegistry{}}).SwitchModel(context.Background(), SwitchModelCommand{
		SessionID: "session-1",
		ModelID:   "missing",
	})
	if err == nil || err.Error() != "model not found: missing" {
		t.Fatalf("expected missing model error, got %v", err)
	}
}

func TestSettingsServiceSetPermissionMode(t *testing.T) {
	memory := settingsMemory("session-1")

	current, err := (SettingsService{Memory: memory}).SetPermissionMode(context.Background(), SetPermissionModeCommand{
		SessionID: "session-1",
		Mode:      string(session.PermissionModeFull),
	})
	if err != nil {
		t.Fatal(err)
	}
	if current.PermissionMode != session.PermissionModeFull {
		t.Fatalf("expected permission mode to be updated, got %#v", current.PermissionMode)
	}
}

func TestSettingsServiceRejectsUnsupportedPermissionMode(t *testing.T) {
	_, err := (SettingsService{Memory: settingsMemory("session-1")}).SetPermissionMode(context.Background(), SetPermissionModeCommand{
		SessionID: "session-1",
		Mode:      "invalid",
	})
	if err == nil {
		t.Fatal("expected unsupported permission mode error")
	}
}

func TestSettingsServiceSetAgentMode(t *testing.T) {
	memory := settingsMemory("session-1")

	current, err := (SettingsService{Memory: memory}).SetAgentMode(context.Background(), SetAgentModeCommand{
		SessionID: "session-1",
		Mode:      string(session.AgentModePlan),
	})
	if err != nil {
		t.Fatal(err)
	}
	if current.AgentMode != session.AgentModePlan {
		t.Fatalf("expected agent mode to be updated, got %#v", current.AgentMode)
	}
}

func TestSettingsServiceSetContextWindow(t *testing.T) {
	memory := settingsMemory("session-1")

	current, err := (SettingsService{Memory: memory}).SetContextWindow(context.Background(), SetContextWindowCommand{
		SessionID: "session-1",
		WindowK:   12,
	})
	if err != nil {
		t.Fatal(err)
	}
	if current.ContextWindowK != 12 {
		t.Fatalf("expected context window to be updated, got %d", current.ContextWindowK)
	}
}

func (s *fakeMemoryStore) SwitchModel(modelID string) error {
	s.currentModelID = modelID
	if s.currentID == "" {
		return nil
	}
	current := s.sessions[s.currentID]
	if current != nil {
		current.Model = modelID
	}
	return nil
}

func (s *fakeMemoryStore) SwitchModelForSession(sessionID string, modelID string) error {
	current := s.sessions[sessionID]
	if current == nil {
		return errors.New("session not found")
	}
	current.Model = modelID
	if s.currentID == sessionID {
		s.currentModelID = modelID
	}
	return nil
}

func (s *fakeMemoryStore) SetPermissionModeForSession(sessionID string, mode session.PermissionMode) error {
	current := s.sessions[sessionID]
	if current == nil {
		return errors.New("session not found")
	}
	current.PermissionMode = session.NormalizePermissionMode(mode)
	return nil
}

func (s *fakeMemoryStore) SetAgentModeForSession(sessionID string, mode session.AgentMode) error {
	current := s.sessions[sessionID]
	if current == nil {
		return errors.New("session not found")
	}
	current.AgentMode = session.NormalizeAgentMode(mode)
	return nil
}

func (s *fakeMemoryStore) SetContextWindowKForSession(sessionID string, windowK int) error {
	current := s.sessions[sessionID]
	if current == nil {
		return errors.New("session not found")
	}
	current.ContextWindowK = windowK
	return nil
}

type fakeSettingsModelRegistry map[string]bool

func (r fakeSettingsModelRegistry) GetModel(name string) modelport.ChatModelPort {
	if r.HasModel(name) {
		return fakeSettingsModel{}
	}
	return nil
}

func (r fakeSettingsModelRegistry) HasModel(name string) bool {
	return r[name]
}

func (r fakeSettingsModelRegistry) ListModels() []modelport.ModelInfo {
	return nil
}

type fakeSettingsModel struct{}

func (fakeSettingsModel) Generate(ctx context.Context, request modelport.GenerateRequest) (modelport.ChatResult, error) {
	return modelport.ChatResult{}, nil
}

func settingsMemory(sessionID string) *fakeMemoryStore {
	memory := &fakeMemoryStore{
		sessions:       map[string]*session.Session{},
		currentID:      sessionID,
		currentModelID: "default",
	}
	if sessionID != "" {
		memory.sessions[sessionID] = &session.Session{
			ID:             sessionID,
			Model:          "default",
			AgentMode:      session.AgentModeChat,
			PermissionMode: session.PermissionModeAsk,
		}
	}
	return memory
}
