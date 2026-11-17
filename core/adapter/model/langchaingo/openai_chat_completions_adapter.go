package langchaingo

import (
	"net/http"

	"github.com/tmc/langchaingo/llms/openai"

	domainmodel "myai/core/domain/model"
	corellm "myai/core/llm"
	modelport "myai/core/port/model"
)

// OpenAIChatCompletionsAdapter creates models that speak the OpenAI
// /chat/completions protocol, including compatible providers such as
// DeepSeek and Ollama.
type OpenAIChatCompletionsAdapter struct{}

var _ modelport.ProtocolAdapter = OpenAIChatCompletionsAdapter{}

func (OpenAIChatCompletionsAdapter) Protocol() domainmodel.Protocol {
	return domainmodel.ProtocolOpenAIChatCompletions
}

func (OpenAIChatCompletionsAdapter) ValidateConfig(config modelport.CreationConfig) error {
	return validateOpenAIConfig(config)
}

func (OpenAIChatCompletionsAdapter) CreateModel(config modelport.CreationConfig) (modelport.ChatModelPort, error) {
	return createOpenAIChatModel(config, false)
}

func createOpenAIChatModel(config modelport.CreationConfig, legacyMaxTokens bool) (modelport.ChatModelPort, error) {
	if err := (OpenAIChatCompletionsAdapter{}).ValidateConfig(config); err != nil {
		return nil, err
	}
	token := config.APIKey
	if domainmodel.NormalizeAuthType(config.AuthType) == domainmodel.AuthTypeNone {
		// The SDK requires a nonempty token even for local unauthenticated
		// providers. The transport always strips this placeholder.
		token = "no-auth"
	}
	options := []openai.Option{
		openai.WithToken(token),
		openai.WithBaseURL(config.BaseURL),
		openai.WithModel(config.ModelName),
	}
	var transport http.RoundTripper = assistantReplayTransport{base: modelTransport{base: http.DefaultTransport}}
	transport = generationSettingsTransport{base: transport, legacyMaxTokens: legacyMaxTokens}
	if domainmodel.NormalizeAuthType(config.AuthType) == domainmodel.AuthTypeNone {
		transport = stripAuthorizationTransport{base: transport}
	}
	options = append(options, openai.WithHTTPClient(&http.Client{Transport: transport}))

	model, err := openai.New(options...)
	if err != nil {
		return nil, err
	}
	return &corellm.Model{LlmModel: model, RequireStreamStopReason: true}, nil
}

type stripAuthorizationTransport struct {
	base http.RoundTripper
}

func (t stripAuthorizationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Del("Authorization")
	return t.base.RoundTrip(clone)
}
