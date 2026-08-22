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

func TestChatAggregatesAllResponseChoices(t *testing.T) {
	captured := &capturingLLM{response: &llms.ContentResponse{Choices: []*llms.ContentChoice{
		{Content: "answer ", ReasoningContent: "think ", GenerationInfo: map[string]any{"PromptTokens": 4, "TotalTokens": 9}},
		{Content: "continued", ReasoningContent: "more", GenerationInfo: map[string]any{"CompletionTokens": 5}, ToolCalls: []llms.ToolCall{{
			ID: "call-1", Type: "function", FunctionCall: &llms.FunctionCall{Name: "read_file", Arguments: `{"path":"a.txt"}`},
		}}},
	}}}
	model := &Model{LlmModel: captured}

	result, err := model.ChatCtx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "answer continued" || result.Reasoning != "think more" {
		t.Fatalf("unexpected aggregate result: %#v", result)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "read_file" {
		t.Fatalf("unexpected tool calls: %#v", result.ToolCalls)
	}
	if result.Usage.PromptTokens != 4 || result.Usage.CompletionTokens != 5 || result.Usage.TotalTokens != 9 || !result.Usage.Available {
		t.Fatalf("unexpected usage: %#v", result.Usage)
	}
}

func TestChatMapsProviderSpecificUsageFields(t *testing.T) {
	type mistralUsage struct {
		PromptTokens     int
		CompletionTokens int
		TotalTokens      int
	}
	captured := &capturingLLM{response: &llms.ContentResponse{Choices: []*llms.ContentChoice{
		{GenerationInfo: map[string]any{
			"InputTokens": 7, "OutputTokens": 3, "CacheReadInputTokens": 2,
		}},
		{GenerationInfo: map[string]any{
			"usage": mistralUsage{PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10},
		}},
	}}}
	result, err := (&Model{LlmModel: captured}).ChatCtx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Usage.PromptTokens != 7 || result.Usage.CompletionTokens != 3 || result.Usage.TotalTokens != 10 || result.Usage.PromptCachedTokens != 2 {
		t.Fatalf("unexpected provider usage mapping: %#v", result.Usage)
	}
}

type capturingLLM struct {
	options  llms.CallOptions
	response *llms.ContentResponse
}

func (m *capturingLLM) GenerateContent(_ context.Context, _ []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	for _, option := range options {
		option(&m.options)
	}
	if m.response != nil {
		return m.response, nil
	}
	return &llms.ContentResponse{}, nil
}

func (m *capturingLLM) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", nil
}
