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

func (OpenAIChatCompletionsAdapter) CreateModel(config modelport.CreationConfig) (modelport.ChatModelPort, error) {
	options := []openai.Option{
		openai.WithToken(config.APIKey),
		openai.WithBaseURL(config.BaseURL),
		openai.WithModel(config.ModelName),
	}
	if domainmodel.NormalizeAuthType(config.AuthType) == domainmodel.AuthTypeNone {
		options = append(options, openai.WithHTTPClient(&http.Client{
			Transport: stripAuthorizationTransport{base: http.DefaultTransport},
		}))
	}

	model, err := openai.New(options...)
	if err != nil {
		return nil, err
	}
	return &corellm.Model{LlmModel: model}, nil
}

type stripAuthorizationTransport struct {
	base http.RoundTripper
}

func (t stripAuthorizationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Del("Authorization")
	return t.base.RoundTrip(clone)
}
