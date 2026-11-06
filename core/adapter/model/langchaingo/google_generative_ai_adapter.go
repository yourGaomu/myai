package langchaingo

import (
	"context"
	"net/http"

	"github.com/tmc/langchaingo/llms/googleai"
	"google.golang.org/api/option"
	googlehttp "google.golang.org/api/transport/http"

	domainmodel "myai/core/domain/model"
	corellm "myai/core/llm"
	modelport "myai/core/port/model"
)

// GoogleGenerativeAIAdapter creates Gemini models through Google's native
// Generative AI API. The current LangChainGo client uses Google's standard
// endpoint; BaseURL is retained in model metadata for a consistent config
// shape but is not used as a custom HTTP endpoint by this SDK.
type GoogleGenerativeAIAdapter struct{}

var _ modelport.ProtocolAdapter = GoogleGenerativeAIAdapter{}

func (GoogleGenerativeAIAdapter) Protocol() domainmodel.Protocol {
	return domainmodel.ProtocolGoogleGenerativeAI
}

func (GoogleGenerativeAIAdapter) ValidateConfig(config modelport.CreationConfig) error {
	return validateGoogleConfig(config)
}

func (GoogleGenerativeAIAdapter) CreateModel(config modelport.CreationConfig) (modelport.ChatModelPort, error) {
	if err := (GoogleGenerativeAIAdapter{}).ValidateConfig(config); err != nil {
		return nil, err
	}
	transport, err := googlehttp.NewTransport(context.Background(), http.DefaultTransport, option.WithAPIKey(config.APIKey))
	if err != nil {
		return nil, err
	}
	model, err := googleai.New(
		context.Background(),
		googleai.WithAPIKey(config.APIKey),
		googleai.WithHTTPClient(&http.Client{Transport: modelTransport{base: transport}}),
		googleai.WithDefaultModel(config.ModelName),
	)
	if err != nil {
		return nil, err
	}
	return &corellm.Model{LlmModel: model, RequireStreamStopReason: true}, nil
}
