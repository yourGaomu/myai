package langchaingo

import (
	"context"
	"testing"

	domainmodel "myai/core/domain/model"
	modelport "myai/core/port/model"
)

func TestFactoryResolvesAdapterByProtocol(t *testing.T) {
	adapter := &recordingAdapter{protocol: domainmodel.Protocol("custom")}
	factory := NewFactory(adapter)

	if !factory.SupportsProtocol(" custom ") {
		t.Fatal("expected custom protocol to be supported")
	}
	if _, err := factory.CreateModel(modelport.CreationConfig{Protocol: "custom"}); err != nil {
		t.Fatal(err)
	}
	if !adapter.called {
		t.Fatal("expected the registered adapter to create the model")
	}
}

func TestFactoryRejectsUnregisteredProtocol(t *testing.T) {
	factory := NewFactory(&recordingAdapter{protocol: "custom"})

	_, err := factory.CreateModel(modelport.CreationConfig{Protocol: "unknown"})
	if err == nil || err.Error() != "unsupported model protocol: unknown" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestZeroValueFactoryKeepsOpenAICompatibility(t *testing.T) {
	var factory Factory
	if !factory.SupportsProtocol(domainmodel.ProtocolOpenAIChatCompletions) {
		t.Fatal("expected zero-value factory to register the built-in adapter")
	}
}

func TestFactoryValidatesBuiltInProtocolConfigurations(t *testing.T) {
	factory := NewFactory()
	cases := []struct {
		name   string
		config modelport.CreationConfig
	}{
		{
			name: "openai-compatible",
			config: modelport.CreationConfig{
				Protocol: domainmodel.ProtocolOpenAIChatCompletions,
				AuthType: domainmodel.AuthTypeBearer,
				APIKey:   "key",
				BaseURL:  "https://api.example.test/v1",
			},
		},
		{
			name: "anthropic-default-endpoint",
			config: modelport.CreationConfig{
				Protocol: domainmodel.ProtocolAnthropicMessages,
				AuthType: domainmodel.AuthTypeBearer,
				APIKey:   "key",
			},
		},
		{
			name: "google-default-endpoint",
			config: modelport.CreationConfig{
				Protocol: domainmodel.ProtocolGoogleGenerativeAI,
				AuthType: domainmodel.AuthTypeBearer,
				APIKey:   "key",
			},
		},
		{
			name: "mistral-default-endpoint",
			config: modelport.CreationConfig{
				Protocol: domainmodel.ProtocolMistralChat,
				AuthType: domainmodel.AuthTypeBearer,
				APIKey:   "key",
			},
		},
		{
			name: "ollama",
			config: modelport.CreationConfig{
				Protocol: domainmodel.ProtocolOllamaChat,
				AuthType: domainmodel.AuthTypeNone,
				BaseURL:  "http://127.0.0.1:11434",
			},
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if err := factory.ValidateConfig(item.config); err != nil {
				t.Fatalf("ValidateConfig() error = %v", err)
			}
		})
	}
}

func TestFactoryRejectsProtocolSpecificConfigurationErrors(t *testing.T) {
	factory := NewFactory()
	cases := []struct {
		name   string
		config modelport.CreationConfig
	}{
		{
			name: "google custom base url",
			config: modelport.CreationConfig{
				Protocol: domainmodel.ProtocolGoogleGenerativeAI,
				AuthType: domainmodel.AuthTypeBearer,
				APIKey:   "key",
				BaseURL:  "https://proxy.example.test",
			},
		},
		{
			name: "ollama openai path",
			config: modelport.CreationConfig{
				Protocol: domainmodel.ProtocolOllamaChat,
				AuthType: domainmodel.AuthTypeNone,
				BaseURL:  "http://127.0.0.1:11434/v1",
			},
		},
		{
			name: "anthropic without key",
			config: modelport.CreationConfig{
				Protocol: domainmodel.ProtocolAnthropicMessages,
				AuthType: domainmodel.AuthTypeBearer,
			},
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if err := factory.ValidateConfig(item.config); err == nil {
				t.Fatal("expected protocol-specific validation error")
			}
		})
	}
}

type recordingAdapter struct {
	protocol domainmodel.Protocol
	called   bool
}

func (a *recordingAdapter) Protocol() domainmodel.Protocol {
	return a.protocol
}

func (a *recordingAdapter) ValidateConfig(modelport.CreationConfig) error {
	return nil
}

func (a *recordingAdapter) CreateModel(modelport.CreationConfig) (modelport.ChatModelPort, error) {
	a.called = true
	return recordingModel{}, nil
}

type recordingModel struct{}

func (recordingModel) Generate(context.Context, modelport.GenerateRequest) (modelport.ChatResult, error) {
	return modelport.ChatResult{}, nil
}
