package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/tmc/langchaingo/llms"

	modelport "myai/core/port/model"
)

var (
	ErrEmptyResponse      = errors.New("model returned no choices")
	ErrIncompleteResponse = errors.New("model stream ended without a stop reason")
	ErrIncompleteToolCall = modelport.ErrIncompleteToolCall
)

func resultFromResponse(response *llms.ContentResponse, streaming bool) (ChatResult, error) {
	if response == nil {
		return ChatResult{}, ErrEmptyResponse
	}
	validChoices := false
	for _, choice := range response.Choices {
		validChoices = validChoices || choice != nil
	}
	if !validChoices {
		return ChatResult{}, ErrEmptyResponse
	}
	result := ChatResult{
		Content: contentFromResponse(response), Reasoning: reasoningFromResponse(response),
		Usage: tokenUsageFromResponse(response), StopReason: stopReasonFromResponse(response),
	}
	// Validate every choice before exposing any calls. Even valid JSON may be a
	// prematurely closed prefix when the provider reports a token limit.
	for _, choice := range response.Choices {
		if choice == nil || len(choice.ToolCalls) == 0 && choice.FuncCall == nil {
			continue
		}
		reason := stopReasonFromChoice(choice)
		if isTruncatedStopReason(reason) || streaming && reason == "" {
			return result, fmt.Errorf("%w: generation did not complete", ErrIncompleteToolCall)
		}
		for _, call := range choice.ToolCalls {
			if !completeFunctionCall(call.FunctionCall) {
				return result, ErrIncompleteToolCall
			}
		}
		if choice.FuncCall != nil && !completeFunctionCall(choice.FuncCall) {
			return result, ErrIncompleteToolCall
		}
	}
	if streaming && result.Content == "" && result.Reasoning == "" && result.StopReason == "" && len(toolCallsFromResponse(response)) == 0 {
		return result, ErrEmptyResponse
	}
	result.ToolCalls = toolCallsFromResponse(response)
	return result, nil
}

func completeFunctionCall(call *llms.FunctionCall) bool {
	if call == nil || strings.TrimSpace(call.Name) == "" {
		return false
	}
	arguments := strings.TrimSpace(call.Arguments)
	// Tool parameters are JSON objects, including {} for no-argument tools.
	return strings.HasPrefix(arguments, "{") && json.Valid([]byte(arguments))
}

func stopReasonFromResponse(response *llms.ContentResponse) string {
	if response == nil {
		return ""
	}
	var reason string
	for _, choice := range response.Choices {
		current := stopReasonFromChoice(choice)
		if isTruncatedStopReason(current) {
			return current
		}
		if reason == "" {
			reason = current
		}
	}
	return reason
}

func stopReasonFromChoice(choice *llms.ContentChoice) string {
	if choice == nil {
		return ""
	}
	if reason := knownStopReason(choice.StopReason); reason != "" {
		return reason
	}
	for _, key := range []string{"StopReason", "FinishReason", "stop_reason", "finish_reason", "done_reason"} {
		if value := choice.GenerationInfo[key]; value != nil {
			// Accept provider string aliases without coercing numbers or objects.
			reflected := reflect.ValueOf(value)
			if reflected.Kind() == reflect.String {
				if reason := knownStopReason(reflected.String()); reason != "" {
					return reason
				}
			}
		}
	}
	return ""
}

func knownStopReason(reason string) string {
	reason = strings.TrimSpace(reason)
	switch strings.ToLower(reason) {
	case "", "null", "none", "unspecified", "finishreasonunspecified", "finish_reason_unspecified":
		return ""
	default:
		return reason
	}
}

func isTruncatedStopReason(reason string) bool {
	normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(reason))
	switch normalized {
	case "length", "maxtokens", "maxoutputtokens", "tokenlimit", "finishreasonmaxtokens", "modelcontextwindowexceeded":
		return true
	default:
		return false
	}
}
