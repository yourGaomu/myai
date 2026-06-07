package llm

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/tmc/langchaingo/llms"

	tooldef "myai/core/tool/tool"
)

type Model struct {
	LlmModel llms.Model
}

type TokenUsage struct {
	PromptTokens       int
	CompletionTokens   int
	TotalTokens        int
	ReasoningTokens    int
	Available          bool
	PromptCachedTokens int
}

func (u TokenUsage) Add(next TokenUsage) TokenUsage {
	return TokenUsage{
		PromptTokens:       u.PromptTokens + next.PromptTokens,
		CompletionTokens:   u.CompletionTokens + next.CompletionTokens,
		TotalTokens:        u.TotalTokens + next.TotalTokens,
		ReasoningTokens:    u.ReasoningTokens + next.ReasoningTokens,
		Available:          u.Available || next.Available,
		PromptCachedTokens: u.PromptCachedTokens + next.PromptCachedTokens,
	}
}

type ChatResult struct {
	Content   string
	Reasoning string
	Usage     TokenUsage
	ToolCalls []llms.ToolCall
}

type ChatStreamHandler struct {
	OnReasoning func(text string)
	OnAnswer    func(text string)
	OnToolCall  func(name string, arguments string)
	OnToolAsk   func(request ToolPermissionRequest) bool
}

type ToolPermissionRequest struct {
	Name       string
	Arguments  string
	Permission tooldef.Permission
	Mode       string
}

func (m *Model) ChatWithStream(mes []llms.MessageContent) (ChatResult, error) {
	return m.ChatWithStreamHandler(mes, ChatStreamHandler{})
}

func (m *Model) ChatWithStreamHandler(mes []llms.MessageContent, handler ChatStreamHandler) (ChatResult, error) {
	return m.ChatWithStreamToolsHandler(mes, nil, handler)
}

func (m *Model) ChatWithStreamTools(mes []llms.MessageContent, tools []llms.Tool) (ChatResult, error) {
	return m.ChatWithStreamToolsHandler(mes, tools, ChatStreamHandler{})
}

func (m *Model) ChatWithStreamToolsHandler(mes []llms.MessageContent, tools []llms.Tool, handler ChatStreamHandler) (ChatResult, error) {
	var builder strings.Builder
	var reasoningBuilder strings.Builder
	streamed := false
	callOptions := []llms.CallOption{
		llms.WithTemperature(0.7),
		llms.WithMaxTokens(2048),
		llms.WithStreamingReasoningFunc(func(ctx context.Context, reasoningChunk, chunk []byte) error {
			if len(reasoningChunk) > 0 {
				streamed = true
				text := string(reasoningChunk)
				reasoningBuilder.WriteString(text)
				if handler.OnReasoning != nil {
					handler.OnReasoning(text)
				}
			}

			if len(chunk) > 0 {
				if len(tools) > 0 && isToolCallChunk(chunk) {
					return nil
				}

				streamed = true

				text := string(chunk)
				builder.WriteString(text)
				if handler.OnAnswer != nil {
					handler.OnAnswer(text)
				}
			}
			return nil
		}),
	}
	if len(tools) > 0 {
		callOptions = append(callOptions, llms.WithTools(tools), llms.WithToolChoice("auto"))
	}

	resp, err := m.LlmModel.GenerateContent(context.Background(), mes, callOptions...)
	if err != nil {
		return ChatResult{}, err
	}

	toolCalls := toolCallsFromResponse(resp)
	if len(toolCalls) > 0 {
		return ChatResult{
			Content:   builder.String(),
			Reasoning: reasoningBuilder.String(),
			Usage:     tokenUsageFromResponse(resp),
			ToolCalls: toolCalls,
		}, nil
	}

	if streamed {
		return ChatResult{
			Content:   builder.String(),
			Reasoning: reasoningBuilder.String(),
			Usage:     tokenUsageFromResponse(resp),
			ToolCalls: toolCalls,
		}, nil
	}

	if resp == nil || len(resp.Choices) == 0 || resp.Choices[0] == nil {
		return ChatResult{}, nil
	}

	text := resp.Choices[0].Content

	reasoning := reasoningFromResponse(resp)
	if reasoning != "" {
		if handler.OnReasoning != nil {
			handler.OnReasoning(reasoning)
		}
	}

	if text != "" {
		if handler.OnAnswer != nil {
			handler.OnAnswer(text)
		}
	}

	return ChatResult{
		Content:   text,
		Reasoning: reasoning,
		Usage:     tokenUsageFromResponse(resp),
		ToolCalls: toolCallsFromResponse(resp),
	}, nil
}

func (m *Model) Chat(mes []llms.MessageContent) (ChatResult, error) {
	resp, err := m.LlmModel.GenerateContent(
		context.Background(),
		mes,
		llms.WithTemperature(0.7),
		llms.WithMaxTokens(2048),
	)
	if err != nil {
		return ChatResult{}, err
	}
	if resp == nil || len(resp.Choices) == 0 || resp.Choices[0] == nil {
		return ChatResult{}, nil
	}

	return ChatResult{
		Content:   resp.Choices[0].Content,
		Reasoning: reasoningFromResponse(resp),
		Usage:     tokenUsageFromResponse(resp),
		ToolCalls: toolCallsFromResponse(resp),
	}, nil
}

func reasoningFromResponse(resp *llms.ContentResponse) string {
	if resp == nil || len(resp.Choices) == 0 || resp.Choices[0] == nil {
		return ""
	}

	if resp.Choices[0].ReasoningContent != "" {
		return resp.Choices[0].ReasoningContent
	}

	if value, ok := resp.Choices[0].GenerationInfo["ThinkingContent"].(string); ok {
		return value
	}

	return ""
}

func tokenUsageFromResponse(resp *llms.ContentResponse) TokenUsage {
	if resp == nil || len(resp.Choices) == 0 || resp.Choices[0] == nil {
		return TokenUsage{}
	}

	info := resp.Choices[0].GenerationInfo
	if info == nil {
		return TokenUsage{}
	}

	usage := TokenUsage{
		PromptTokens:       intFromGenerationInfo(info["PromptTokens"]),
		CompletionTokens:   intFromGenerationInfo(info["CompletionTokens"]),
		TotalTokens:        intFromGenerationInfo(info["TotalTokens"]),
		ReasoningTokens:    intFromGenerationInfo(info["ReasoningTokens"]),
		PromptCachedTokens: intFromGenerationInfo(info["PromptCachedTokens"]),
	}

	_, hasPrompt := info["PromptTokens"]
	_, hasCompletion := info["CompletionTokens"]
	_, hasTotal := info["TotalTokens"]
	usage.Available = hasPrompt || hasCompletion || hasTotal

	return usage
}

func toolCallsFromResponse(resp *llms.ContentResponse) []llms.ToolCall {
	if resp == nil || len(resp.Choices) == 0 || resp.Choices[0] == nil {
		return nil
	}

	return resp.Choices[0].ToolCalls
}

func isToolCallChunk(chunk []byte) bool {
	var toolCalls []struct {
		ID       string         `json:"id"`
		Type     string         `json:"type"`
		Function map[string]any `json:"function"`
	}
	if err := json.Unmarshal(chunk, &toolCalls); err != nil {
		return false
	}
	if len(toolCalls) == 0 {
		return false
	}

	for _, call := range toolCalls {
		if call.ID != "" || call.Type != "" || call.Function != nil {
			return true
		}
	}

	return false
}

func intFromGenerationInfo(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int8:
		return int(v)
	case int16:
		return int(v)
	case int32:
		return int(v)
	case int64:
		return int(v)
	case uint:
		return int(v)
	case uint8:
		return int(v)
	case uint16:
		return int(v)
	case uint32:
		return int(v)
	case uint64:
		return int(v)
	case float32:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}
