package langchaingo

import (
	"strings"

	domainmodel "myai/core/domain/model"
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

func (adapter MistralChatAdapter) CreateModel(config modelport.CreationConfig) (modelport.ChatModelPort, error) {
	if err := adapter.ValidateConfig(config); err != nil {
		return nil, err
	}
	// The native SDK creates requests without ctx and performs hidden retries.
	// Mistral speaks chat/completions, so reuse the existing compatible client
	// to preserve cancellation and the shared retry budget.
	if config.BaseURL == "" {
		config.BaseURL = "https://api.mistral.ai"
	}
	config.BaseURL = strings.TrimRight(config.BaseURL, "/")
	if !strings.HasSuffix(config.BaseURL, "/v1") {
		config.BaseURL += "/v1"
	}
	if config.ModelName == "" {
		config.ModelName = "open-mistral-7b"
	}
	return createOpenAIChatModel(config, adapter.Protocol())
}
