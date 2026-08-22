package langchaingo

import (
	"github.com/tmc/langchaingo/llms/ollama"

	domainmodel "myai/core/domain/model"
	corellm "myai/core/llm"
	modelport "myai/core/port/model"
)

// OllamaChatAdapter creates models through Ollama's native /api/chat API.
// Unlike the OpenAI-compatible adapter, BaseURL must be the Ollama server
// root, for example http://127.0.0.1:11434.
type OllamaChatAdapter struct{}

var _ modelport.ProtocolAdapter = OllamaChatAdapter{}

func (OllamaChatAdapter) Protocol() domainmodel.Protocol {
	return domainmodel.ProtocolOllamaChat
}

func (OllamaChatAdapter) ValidateConfig(config modelport.CreationConfig) error {
	return validateOllamaConfig(config)
}

func (OllamaChatAdapter) CreateModel(config modelport.CreationConfig) (modelport.ChatModelPort, error) {
	if err := (OllamaChatAdapter{}).ValidateConfig(config); err != nil {
		return nil, err
	}
	model, err := ollama.New(
		ollama.WithServerURL(config.BaseURL),
		ollama.WithModel(config.ModelName),
	)
	if err != nil {
		return nil, err
	}
	return &corellm.Model{LlmModel: model}, nil
}
