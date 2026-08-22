package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	modelcommand "myai/core/application/model/command"
	domainmodel "myai/core/domain/model"
	modelport "myai/core/port/model"
	repository "myai/core/port/repository"
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

func TestConfigServiceAddConfigAllowsCustomProviderAndRequiresSupportedProtocol(t *testing.T) {
	service := ConfigService{
		Repository: &fakeConfigRepository{},
		Registry:   &fakeModelRegistry{},
		Factory:    &fakeModelFactory{model: fakeChatModel{}},
	}
	result, err := service.AddConfig(context.Background(), modelcommand.AddConfig{
		ID:        "deepseek-chat",
		Provider:  "deepseek",
		BaseURL:   "https://api.deepseek.test/v1",
		APIKey:    "secret",
		ModelName: "deepseek-chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Config.Provider != "deepseek" || result.Config.Protocol != domainmodel.ProtocolOpenAIChatCompletions {
		t.Fatalf("unexpected custom provider config: %#v", result.Config)
	}

	_, err = (ConfigService{
		Repository: &fakeConfigRepository{},
		Registry:   &fakeModelRegistry{},
		Factory:    &fakeModelFactory{model: fakeChatModel{}},
	}).AddConfig(context.Background(), modelcommand.AddConfig{
		ID:        "gpt-test",
		Provider:  "custom",
		Protocol:  "custom-protocol",
		BaseURL:   "https://example.test",
		APIKey:    "secret",
		ModelName: "gpt-test",
	})

	if err == nil || err.Error() != "unsupported model protocol: custom-protocol" {
		t.Fatalf("expected unsupported protocol error, got %v", err)
	}
}

func TestConfigServiceAddConfigAllowsNoAuthWithoutAPIKey(t *testing.T) {
	result, err := (ConfigService{
		Repository: &fakeConfigRepository{},
		Registry:   &fakeModelRegistry{},
		Factory:    &fakeModelFactory{model: fakeChatModel{}},
	}).AddConfig(context.Background(), modelcommand.AddConfig{
		ID:        "ollama-local",
		Provider:  "ollama",
		AuthType:  domainmodel.AuthTypeNone,
		BaseURL:   "http://127.0.0.1:11434/v1/",
		ModelName: "qwen3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Config.BaseURL != "http://127.0.0.1:11434/v1" || result.Config.AuthType != domainmodel.AuthTypeNone {
		t.Fatalf("unexpected no-auth config: %#v", result.Config)
	}
}

func TestConfigServiceAddConfigRejectsFullChatCompletionsURL(t *testing.T) {
	_, err := (ConfigService{
		Repository: &fakeConfigRepository{},
		Registry:   &fakeModelRegistry{},
		Factory:    &fakeModelFactory{model: fakeChatModel{}},
	}).AddConfig(context.Background(), modelcommand.AddConfig{
		ID:        "invalid-url",
		AuthType:  domainmodel.AuthTypeNone,
		BaseURL:   "https://example.test/v1/chat/completions",
		ModelName: "model",
	})
	if err == nil || err.Error() != "base url must be the API root and cannot include /chat/completions" {
		t.Fatalf("expected API root error, got %v", err)
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

func (r *fakeConfigRepository) GetConfig(ctx context.Context, id string) (domainmodel.Config, error) {
	if r.saved.ID == id {
		return r.saved, nil
	}
	return domainmodel.Config{}, repository.ErrNotFound
}

func (r *fakeConfigRepository) ListConfigs(ctx context.Context) ([]domainmodel.Config, error) {
	if r.saved.ID == "" {
		return nil, nil
	}
	return []domainmodel.Config{r.saved}, nil
}

func (r *fakeConfigRepository) DeleteConfig(ctx context.Context, id string) error {
	if r.saved.ID != id {
		return repository.ErrNotFound
	}
	r.saved = domainmodel.Config{}
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

func (r *fakeModelRegistry) RemoveModel(modelName string) bool {
	if r.models == nil {
		return false
	}
	if _, ok := r.models[modelName]; !ok {
		return false
	}
	delete(r.models, modelName)
	delete(r.infos, modelName)
	return true
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

func (f *fakeModelFactory) SupportsProtocol(protocol domainmodel.Protocol) bool {
	return domainmodel.NormalizeProtocol(protocol) == domainmodel.ProtocolOpenAIChatCompletions
}

func (f *fakeModelFactory) ValidateConfig(config modelport.CreationConfig) error {
	if !f.SupportsProtocol(config.Protocol) {
		return errors.New("unsupported model protocol: " + string(config.Protocol))
	}
	if config.BaseURL == "" {
		return errors.New("base url is empty")
	}
	if strings.HasSuffix(strings.ToLower(strings.TrimRight(config.BaseURL, "/")), "/chat/completions") {
		return errors.New("base url must be the API root and cannot include /chat/completions")
	}
	if config.AuthType == domainmodel.AuthTypeBearer && config.APIKey == "" {
		return errors.New("api key is empty")
	}
	return nil
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
