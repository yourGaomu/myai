package service

import (
	"context"
	"errors"
	"testing"
	"time"

	modelcommand "myai/core/application/model/command"
	domainmodel "myai/core/domain/model"
	modelport "myai/core/port/model"
)

func TestConfigServiceAddConfigNormalizesPersistsAndRegistersModel(t *testing.T) {
	repo := &fakeConfigRepository{}
	registry := &fakeModelRegistry{}
	factory := &fakeModelFactory{model: fakeChatModel{}}

	result, err := (ConfigService{
		Repository: repo,
		Registry:   registry,
		Factory:    factory,
		Now:        fixedModelTime,
	}).AddConfig(context.Background(), modelcommand.AddConfig{
		ID:       "gpt-test",
		Name:     " Test Model ",
		Provider: " OpenAI-Compatible ",
		BaseURL:  " https://example.test ",
		APIKey:   " secret ",
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Config.Provider != "openai-compatible" || result.Config.ModelName != "gpt-test" || !result.Config.Enabled {
		t.Fatalf("unexpected normalized config: %#v", result.Config)
	}
	if repo.saved.ID != "gpt-test" || !repo.saved.UpdatedAt.Equal(fixedModelTime()) {
		t.Fatalf("expected config to be saved with timestamps, got %#v", repo.saved)
	}
	if factory.config.ModelName != "gpt-test" || factory.config.APIKey != "secret" {
		t.Fatalf("expected factory to receive normalized config, got %#v", factory.config)
	}
	if registry.infos["gpt-test"].Name != "Test Model" {
		t.Fatalf("expected registered model info, got %#v", registry.infos["gpt-test"])
	}
}

func TestConfigServiceAddConfigRejectsDuplicateModel(t *testing.T) {
	_, err := (ConfigService{
		Repository: &fakeConfigRepository{},
		Registry:   &fakeModelRegistry{models: map[string]modelport.ChatModelPort{"gpt-test": fakeChatModel{}}},
		Factory:    &fakeModelFactory{model: fakeChatModel{}},
	}).AddConfig(context.Background(), modelcommand.AddConfig{
		ID:        "gpt-test",
		Provider:  "openai",
		BaseURL:   "https://example.test",
		APIKey:    "secret",
		ModelName: "gpt-test",
	})

	if err == nil || err.Error() != "model already exists: gpt-test" {
		t.Fatalf("expected duplicate model error, got %v", err)
	}
}

func TestConfigServiceAddConfigRequiresSupportedProvider(t *testing.T) {
	_, err := (ConfigService{
		Repository: &fakeConfigRepository{},
		Registry:   &fakeModelRegistry{},
		Factory:    &fakeModelFactory{model: fakeChatModel{}},
	}).AddConfig(context.Background(), modelcommand.AddConfig{
		ID:        "gpt-test",
		Provider:  "other",
		BaseURL:   "https://example.test",
		APIKey:    "secret",
		ModelName: "gpt-test",
	})

	if err == nil || err.Error() != "unsupported provider: other" {
		t.Fatalf("expected unsupported provider error, got %v", err)
	}
}

func TestConfigServiceAddConfigDoesNotSaveWhenFactoryFails(t *testing.T) {
	repo := &fakeConfigRepository{}

	_, err := (ConfigService{
		Repository: repo,
		Registry:   &fakeModelRegistry{},
		Factory:    &fakeModelFactory{err: errors.New("factory failed")},
	}).AddConfig(context.Background(), modelcommand.AddConfig{
		ID:        "gpt-test",
		Provider:  "openai",
		BaseURL:   "https://example.test",
		APIKey:    "secret",
		ModelName: "gpt-test",
	})

	if err == nil || err.Error() != "factory failed" {
		t.Fatalf("expected factory error, got %v", err)
	}
	if repo.saved.ID != "" {
		t.Fatalf("expected config not to be saved, got %#v", repo.saved)
	}
}

type fakeConfigRepository struct {
	saved domainmodel.Config
	err   error
}

func (r *fakeConfigRepository) SaveConfig(ctx context.Context, model domainmodel.Config) error {
	if r.err != nil {
		return r.err
	}
	r.saved = model
	return nil
}

type fakeModelRegistry struct {
	models map[string]modelport.ChatModelPort
	infos  map[string]modelport.ModelInfo
}

func (r *fakeModelRegistry) GetModel(name string) modelport.ChatModelPort {
	if r.models == nil {
		return nil
	}
	return r.models[name]
}

func (r *fakeModelRegistry) HasModel(name string) bool {
	return r.GetModel(name) != nil
}

func (r *fakeModelRegistry) ListModels() []modelport.ModelInfo {
	if r.infos == nil {
		return nil
	}
	models := make([]modelport.ModelInfo, 0, len(r.infos))
	for _, info := range r.infos {
		models = append(models, info)
	}
	return models
}

func (r *fakeModelRegistry) SetModelInfo(modelName string, model modelport.ChatModelPort, info modelport.ModelInfo) {
	if r.models == nil {
		r.models = map[string]modelport.ChatModelPort{}
	}
	if r.infos == nil {
		r.infos = map[string]modelport.ModelInfo{}
	}
	r.models[modelName] = model
	r.infos[modelName] = info
}

type fakeModelFactory struct {
	config modelport.CreationConfig
	model  modelport.ChatModelPort
	err    error
}

func (f *fakeModelFactory) CreateModel(config modelport.CreationConfig) (modelport.ChatModelPort, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.config = config
	return f.model, nil
}

type fakeChatModel struct{}

func (fakeChatModel) Generate(ctx context.Context, request modelport.GenerateRequest) (modelport.ChatResult, error) {
	return modelport.ChatResult{}, nil
}

func validModelConfig(id string) domainmodel.Config {
	return domainmodel.Config{
		ID:        id,
		Provider:  "openai",
		BaseURL:   "https://example.test",
		APIKey:    "secret",
		ModelName: id,
		Enabled:   true,
	}
}

func fixedModelTime() time.Time {
	return time.Date(2026, 7, 10, 13, 0, 0, 0, time.UTC)
}
