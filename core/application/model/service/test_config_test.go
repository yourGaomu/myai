package service

import (
	"context"
	"errors"
	"testing"

	modelcommand "myai/core/application/model/command"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
)

func TestConfigServiceTestConfigDoesNotPersistOrRegister(t *testing.T) {
	repository := &fakeConfigRepository{}
	registry := &fakeModelRegistry{}
	factory := &fakeModelFactory{model: testRecordingModel{}}

	result, err := (ConfigService{
		Repository: repository,
		Registry:   registry,
		Factory:    factory,
	}).TestConfig(context.Background(), modelcommand.AddConfig{
		ID:        "test-model",
		BaseURL:   "https://example.test/v1",
		APIKey:    "secret",
		ModelName: "provider-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || repository.saved.ID != "" || registry.HasModel("test-model") {
		t.Fatalf("test config mutated state: result=%#v saved=%#v", result, repository.saved)
	}
	if factory.config.ModelName != "provider-model" {
		t.Fatalf("factory received unexpected config: %#v", factory.config)
	}
}

func TestConfigServiceTestConfigReturnsModelError(t *testing.T) {
	_, err := (ConfigService{
		Factory: &fakeModelFactory{model: testRecordingModel{err: errors.New("connection refused")}},
	}).TestConfig(context.Background(), modelcommand.AddConfig{
		ID:        "test-model",
		BaseURL:   "https://example.test/v1",
		APIKey:    "secret",
		ModelName: "provider-model",
	})
	if err == nil || err.Error() != "connection refused" {
		t.Fatalf("expected model error, got %v", err)
	}
}

type testRecordingModel struct {
	err error
}

func (m testRecordingModel) Generate(_ context.Context, request modelport.GenerateRequest) (modelport.ChatResult, error) {
	if len(request.Messages) != 1 || request.Messages[0].Role != domainmessage.RoleUser || request.Messages[0].Text() != "Reply with OK." {
		return modelport.ChatResult{}, errors.New("unexpected connection test request")
	}
	return modelport.ChatResult{}, m.err
}
