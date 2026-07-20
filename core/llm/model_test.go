package llm

import (
	"context"
	"testing"

	"github.com/tmc/langchaingo/llms"

	generation "myai/core/domain/generation"
	modelport "myai/core/port/model"
)

func TestGenerateMapsResolvedSettingsToCallOptions(t *testing.T) {
	captured := &capturingLLM{}
	model := &Model{LlmModel: captured}
	settings := generation.ResolvedSettings{
		Temperature:     0.2,
		TopP:            0.8,
		MaxOutputTokens: 4096,
	}

	if _, err := model.Generate(context.Background(), modelport.GenerateRequest{Settings: settings}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if captured.options.Temperature != settings.Temperature ||
		captured.options.TopP != settings.TopP ||
		captured.options.MaxTokens != settings.MaxOutputTokens {
		t.Fatalf("call options = %#v", captured.options)
	}
}

type capturingLLM struct {
	options llms.CallOptions
}

func (m *capturingLLM) GenerateContent(_ context.Context, _ []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	for _, option := range options {
		option(&m.options)
	}
	return &llms.ContentResponse{}, nil
}

func (m *capturingLLM) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", nil
}
