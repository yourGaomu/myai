package langchaingo

import (
	"github.com/tmc/langchaingo/llms/mistral"

	domainmodel "myai/core/domain/model"
	corellm "myai/core/llm"
	modelport "myai/core/port/model"
)

// MistralChatAdapter creates models through Mistral's native chat API.
type MistralChatAdapter struct{}

var _ modelport.ProtocolAdapter = MistralChatAdapter{}

func (MistralChatAdapter) Protocol() domainmodel.Protocol {
	return domainmodel.ProtocolMistralChat
}

func (MistralChatAdapter) ValidateConfig(config modelport.CreationConfig) error {
	return validateMistralConfig(config)
}

func (MistralChatAdapter) CreateModel(config modelport.CreationConfig) (modelport.ChatModelPort, error) {
	if err := (MistralChatAdapter{}).ValidateConfig(config); err != nil {
		return nil, err
	}
	options := []mistral.Option{
		mistral.WithAPIKey(config.APIKey),
		mistral.WithModel(config.ModelName),
	}
	if config.BaseURL != "" {
		options = append(options, mistral.WithEndpoint(config.BaseURL))
	}
	model, err := mistral.New(options...)
	if err != nil {
		return nil, err
	}
	return &corellm.Model{LlmModel: model}, nil
}
