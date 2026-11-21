package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/tmc/langchaingo/llms"

	llmmapper "myai/core/adapter/llm/langchaingo"
	generation "myai/core/domain/generation"
	modelport "myai/core/port/model"
)

type Model struct {
	LlmModel llms.Model
	Policy   RequestPolicy
	// Enable when the SDK preserves finish metadata in streaming responses.
	RequireStreamStopReason bool
	// ChatCompletionsImages is set by adapters with a verified image_url mapping.
	ChatCompletionsImages bool
}

var _ modelport.ChatModelPort = (*Model)(nil)
var _ modelport.ImageInputSupport = (*Model)(nil)

type TokenUsage = modelport.TokenUsage
type ChatResult = modelport.ChatResult
type ChatStreamHandler = modelport.ChatStreamHandler
type ToolResultEvent = modelport.ToolResultEvent
type ToolPermissionRequest = modelport.ToolPermissionRequest
type GenerateRequest = modelport.GenerateRequest

func (m *Model) SupportsImageInput() bool {
	return m != nil && m.ChatCompletionsImages
}

func (m *Model) Generate(ctx context.Context, request modelport.GenerateRequest) (modelport.ChatResult, error) {
	if len(request.Images) > 0 && !m.SupportsImageInput() {
		return modelport.ChatResult{}, modelport.ErrImageInputUnsupported
	}
	messages, err := llmmapper.ToLLMRequest(request)
	if err != nil {
		return modelport.ChatResult{}, err
	}
	result, err := m.chatWithStreamToolsHandlerCtx(
		llmmapper.WithRequest(ctx, request),
		messages,
		llmmapper.ToLLMTools(request.Tools),
		request.Stream,
		request.Settings,
	)
	if len(request.Images) > 0 && !modelport.IsContextWindowExceeded(err) {
		var requestErr *RequestError
		if errors.As(err, &requestErr) {
			switch requestErr.StatusCode {
			case http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity:
				return result, fmt.Errorf("%w (HTTP %d)", modelport.ErrImageInputRejected, requestErr.StatusCode)
			}
		}
	}
	return result, err
}

func (m *Model) ChatWithStream(mes []llms.MessageContent) (ChatResult, error) {
	return m.ChatWithStreamCtx(context.Background(), mes)
}

func (m *Model) ChatWithStreamCtx(ctx context.Context, mes []llms.MessageContent) (ChatResult, error) {
	return m.ChatWithStreamHandlerCtx(ctx, mes, ChatStreamHandler{})
}

func (m *Model) ChatWithStreamHandler(mes []llms.MessageContent, handler ChatStreamHandler) (ChatResult, error) {
	return m.ChatWithStreamHandlerCtx(context.Background(), mes, handler)
}

func (m *Model) ChatWithStreamHandlerCtx(ctx context.Context, mes []llms.MessageContent, handler ChatStreamHandler) (ChatResult, error) {
	return m.ChatWithStreamToolsHandlerCtx(ctx, mes, nil, handler)
}

func (m *Model) ChatWithStreamTools(mes []llms.MessageContent, tools []llms.Tool) (ChatResult, error) {
	return m.ChatWithStreamToolsCtx(context.Background(), mes, tools)
}

func (m *Model) ChatWithStreamToolsCtx(ctx context.Context, mes []llms.MessageContent, tools []llms.Tool) (ChatResult, error) {
	return m.ChatWithStreamToolsHandlerCtx(ctx, mes, tools, ChatStreamHandler{})
}

func (m *Model) ChatWithStreamToolsHandler(mes []llms.MessageContent, tools []llms.Tool, handler ChatStreamHandler) (ChatResult, error) {
	return m.ChatWithStreamToolsHandlerCtx(context.Background(), mes, tools, handler)
}

func (m *Model) ChatWithStreamToolsHandlerCtx(ctx context.Context, mes []llms.MessageContent, tools []llms.Tool, handler ChatStreamHandler) (ChatResult, error) {
	return m.chatWithStreamToolsHandlerCtx(ctx, mes, tools, handler, generation.SystemDefaults())
}

func (m *Model) chatWithStreamToolsHandlerCtx(ctx context.Context, mes []llms.MessageContent, tools []llms.Tool, handler ChatStreamHandler, settings generation.ResolvedSettings) (ChatResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := settings.Validate(); err != nil {
		return ChatResult{}, fmt.Errorf("invalid resolved generation settings: %w", err)
	}
	var builder strings.Builder
	var reasoningBuilder strings.Builder
	streamed := false
	streamAnswer := func(ctx context.Context, chunk []byte) error {
		if len(chunk) == 0 {
			return nil
		}
		if len(tools) > 0 && isToolCallChunk(chunk) {
			return nil
		}

		streamed = true

		text := string(chunk)
		builder.WriteString(text)
		if handler.OnAnswer != nil {
			handler.OnAnswer(text)
		}
		return nil
	}
	callOptions := []llms.CallOption{
		llms.WithTemperature(settings.Temperature),
		llms.WithTopP(settings.TopP),
		llms.WithMaxTokens(settings.MaxOutputTokens),
		llms.WithStreamingFunc(streamAnswer),
		llms.WithStreamingReasoningFunc(func(ctx context.Context, reasoningChunk, chunk []byte) error {
			if len(reasoningChunk) > 0 {
				streamed = true
				text := string(reasoningChunk)
				reasoningBuilder.WriteString(text)
				if handler.OnReasoning != nil {
					handler.OnReasoning(text)
				}
			}
			return nil
		}),
	}
	if len(tools) > 0 {
		callOptions = append(callOptions, llms.WithTools(tools), llms.WithToolChoice("auto"))
	}

	resp, err := m.generateContent(ctx, mes, callOptions...)
	if err != nil {
		return ChatResult{Content: builder.String(), Reasoning: reasoningBuilder.String()}, err
	}
	result, err := resultFromResponse(resp, true)
	if err != nil {
		if result.Content == "" {
			result.Content = builder.String()
		}
		if result.Reasoning == "" {
			result.Reasoning = reasoningBuilder.String()
		}
		if err == ErrEmptyResponse && streamed && m.RequireStreamStopReason {
			err = ErrIncompleteResponse
		}
		return result, err
	}
	if m.RequireStreamStopReason && result.StopReason == "" {
		return result, ErrIncompleteResponse
	}
	if streamed {
		// SDK tool deltas can replace a text callback in the same frame. Only
		// deliver the missing suffix from the complete response.
		result.Content = completeStreamText(builder.String(), result.Content, handler.OnAnswer)
		result.Reasoning = completeStreamText(reasoningBuilder.String(), result.Reasoning, handler.OnReasoning)
		return result, nil
	}

	if result.Reasoning != "" && handler.OnReasoning != nil {
		handler.OnReasoning(result.Reasoning)
	}
	if result.Content != "" && handler.OnAnswer != nil {
		handler.OnAnswer(result.Content)
	}
	return result, nil
}

func completeStreamText(streamed, complete string, emit func(string)) string {
	if complete == "" {
		return streamed
	}
	if emit != nil && len(complete) > len(streamed) && strings.HasPrefix(complete, streamed) {
		emit(complete[len(streamed):])
	}
	return complete
}

func (m *Model) Chat(mes []llms.MessageContent) (ChatResult, error) {
	return m.ChatCtx(context.Background(), mes)
}

func (m *Model) ChatCtx(ctx context.Context, mes []llms.MessageContent) (ChatResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	settings := generation.SystemDefaults()
	resp, err := m.generateContent(
		ctx,
		mes,
		llms.WithTemperature(settings.Temperature),
		llms.WithTopP(settings.TopP),
		llms.WithMaxTokens(settings.MaxOutputTokens),
	)
	if err != nil {
		return ChatResult{}, err
	}
	return resultFromResponse(resp, false)
}

func reasoningFromResponse(resp *llms.ContentResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}

	var builder strings.Builder
	for _, choice := range resp.Choices {
		if choice == nil {
			continue
		}
		reasoning := choice.ReasoningContent
		if reasoning == "" {
			if value, ok := choice.GenerationInfo["ThinkingContent"].(string); ok {
				reasoning = value
			}
		}
		builder.WriteString(reasoning)
	}
	return builder.String()
}

func contentFromResponse(resp *llms.ContentResponse) string {
	if resp == nil {
		return ""
	}

	var builder strings.Builder
	for _, choice := range resp.Choices {
		if choice != nil {
			builder.WriteString(choice.Content)
		}
	}
	return builder.String()
}

func tokenUsageFromResponse(resp *llms.ContentResponse) TokenUsage {
	if resp == nil || len(resp.Choices) == 0 {
		return TokenUsage{}
	}

	var usage TokenUsage
	var seenPrompt, seenCompletion, seenTotal, seenReasoning, seenCached bool
	for _, choice := range resp.Choices {
		if choice == nil || choice.GenerationInfo == nil {
			continue
		}
		info := choice.GenerationInfo
		usage.PromptTokens, seenPrompt = mergeGenerationInfo(usage.PromptTokens, seenPrompt, info, "PromptTokens", "InputTokens")
		usage.CompletionTokens, seenCompletion = mergeGenerationInfo(usage.CompletionTokens, seenCompletion, info, "CompletionTokens", "OutputTokens")
		usage.TotalTokens, seenTotal = mergeGenerationInfo(usage.TotalTokens, seenTotal, info, "TotalTokens")
		usage.ReasoningTokens, seenReasoning = mergeGenerationInfo(usage.ReasoningTokens, seenReasoning, info, "ReasoningTokens", "ThinkingTokens")
		usage.PromptCachedTokens, seenCached = mergeGenerationInfo(usage.PromptCachedTokens, seenCached, info, "PromptCachedTokens", "CachedTokens", "CacheReadInputTokens")
		if nested, ok := info["usage"]; ok {
			usage.PromptTokens, seenPrompt = mergeNestedGenerationInfo(usage.PromptTokens, seenPrompt, nested, "PromptTokens")
			usage.CompletionTokens, seenCompletion = mergeNestedGenerationInfo(usage.CompletionTokens, seenCompletion, nested, "CompletionTokens")
			usage.TotalTokens, seenTotal = mergeNestedGenerationInfo(usage.TotalTokens, seenTotal, nested, "TotalTokens")
		}
	}
	if !seenTotal && seenPrompt && seenCompletion {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		seenTotal = true
	}
	usage.Available = seenPrompt || seenCompletion || seenTotal

	return usage
}

func toolCallsFromResponse(resp *llms.ContentResponse) []modelport.ToolCall {
	if resp == nil || len(resp.Choices) == 0 {
		return nil
	}

	var calls []llms.ToolCall
	for _, choice := range resp.Choices {
		if choice != nil {
			calls = append(calls, choice.ToolCalls...)
		}
	}
	return llmmapper.FromLLMToolCalls(calls)
}

func mergeGenerationInfo(current int, seen bool, info map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		value, ok := info[key]
		if !ok {
			continue
		}
		return mergeTokenValue(current, seen, value)
	}
	return current, seen
}

func mergeNestedGenerationInfo(current int, seen bool, value any, field string) (int, bool) {
	reflected := reflect.ValueOf(value)
	for reflected.IsValid() && (reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Interface) {
		if reflected.IsNil() {
			return current, seen
		}
		reflected = reflected.Elem()
	}
	if !reflected.IsValid() || reflected.Kind() != reflect.Struct {
		return current, seen
	}
	fieldValue := reflected.FieldByName(field)
	if !fieldValue.IsValid() || !fieldValue.CanInterface() {
		return current, seen
	}
	return mergeTokenValue(current, seen, fieldValue.Interface())
}

func mergeTokenValue(current int, seen bool, value any) (int, bool) {
	parsed := intFromGenerationInfo(value)
	if parsed != 0 || !seen {
		current = parsed
	}
	return current, true
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
