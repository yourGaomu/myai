package langchaingo

import (
	"context"
	"net/http"
	"strings"

	"google.golang.org/api/option"
	googlehttp "google.golang.org/api/transport/http"

	domainmodel "myai/core/domain/model"
	corellm "myai/core/llm"
	modelport "myai/core/port/model"
)

// GoogleGenerativeAIAdapter uses Gemini's native API at Google's standard endpoint.
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
	return createGoogleGenerativeAIModel(config, http.DefaultTransport, "")
}

func createGoogleGenerativeAIModel(config modelport.CreationConfig, base http.RoundTripper, endpoint string) (*corellm.Model, error) {
	if endpoint == "" {
		endpoint = "https://generativelanguage.googleapis.com"
	}
	transport, err := googlehttp.NewTransport(context.Background(), base, option.WithAPIKey(config.APIKey))
	if err != nil {
		return nil, err
	}
	model := &geminiModel{
		name: config.ModelName, endpoint: strings.TrimRight(endpoint, "/"),
		client: &http.Client{Transport: modelTransport{base: transport}},
	}
	return &corellm.Model{LlmModel: model, RequireStreamStopReason: true}, nil
}
