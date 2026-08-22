package langchaingo

import (
	"github.com/tmc/langchaingo/llms/anthropic"

	domainmodel "myai/core/domain/model"
	corellm "myai/core/llm"
	modelport "myai/core/port/model"
)

// AnthropicMessagesAdapter creates models through Anthropic's native
// Messages API. BaseURL is the Anthropic API root, not an OpenAI endpoint.
type AnthropicMessagesAdapter struct{}

var _ modelport.ProtocolAdapter = AnthropicMessagesAdapter{}

func (AnthropicMessagesAdapter) Protocol() domainmodel.Protocol {
	return domainmodel.ProtocolAnthropicMessages
}

func (AnthropicMessagesAdapter) ValidateConfig(config modelport.CreationConfig) error {
	return validateAnthropicConfig(config)
}

func (AnthropicMessagesAdapter) CreateModel(config modelport.CreationConfig) (modelport.ChatModelPort, error) {
	if err := (AnthropicMessagesAdapter{}).ValidateConfig(config); err != nil {
		return nil, err
	}
	options := []anthropic.Option{
		anthropic.WithToken(config.APIKey),
		anthropic.WithModel(config.ModelName),
	}
	if config.BaseURL != "" {
		options = append(options, anthropic.WithBaseURL(config.BaseURL))
	}
	model, err := anthropic.New(options...)
	if err != nil {
		return nil, err
	}
	return &corellm.Model{LlmModel: model}, nil
}
