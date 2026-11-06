package llm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"

	modelport "myai/core/port/model"
)

func TestModelRejectsEmptyChoices(t *testing.T) {
	for name, response := range map[string]*llms.ContentResponse{
		"nil":        nil,
		"empty":      {},
		"nil choice": {Choices: []*llms.ContentChoice{nil}},
	} {
		for _, streaming := range []bool{false, true} {
			t.Run(name+map[bool]string{false: "/chat", true: "/stream"}[streaming], func(t *testing.T) {
				calls := 0
				model := &Model{LlmModel: functionLLM(func(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
					calls++
					return response, nil
				})}
				var err error
				if streaming {
					_, err = model.ChatWithStreamCtx(context.Background(), nil)
				} else {
					_, err = model.ChatCtx(context.Background(), nil)
				}
				if !errors.Is(err, ErrEmptyResponse) || calls != 1 {
					t.Fatalf("err=%v calls=%d", err, calls)
				}
			})
		}
	}
}

func TestToolArgumentsMustBeCompleteBeforeExposure(t *testing.T) {
	cases := []struct {
		name       string
		stopReason string
		arguments  string
		streaming  bool
		valid      bool
	}{
		{"valid stream", "tool_calls", `{"path":"readme.txt"}`, true, true},
		{"empty object", "tool_use", `{}`, true, true},
		{"valid old nonstream response", "", `{}`, false, true},
		{"unfinished JSON", "tool_calls", `{"path":`, true, false},
		{"unfinished nonstream JSON", "tool_calls", `{"path":`, false, false},
		{"missing arguments", "tool_calls", "", true, false},
		{"null arguments", "tool_calls", `null`, true, false},
		{"array arguments", "tool_calls", `[]`, true, false},
		{"length even with valid JSON", "length", `{}`, true, false},
		{"anthropic limit", "max_tokens", `{}`, false, false},
		{"google limit", "FinishReasonMaxTokens", `{}`, true, false},
		{"ollama limit", "length", `{}`, false, false},
		{"missing stream termination", "", `{}`, true, false},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			model := &Model{LlmModel: &capturingLLM{response: &llms.ContentResponse{Choices: []*llms.ContentChoice{{
				Content: "working", StopReason: item.stopReason,
				ToolCalls: []llms.ToolCall{{ID: "call-1", Type: "function", FunctionCall: &llms.FunctionCall{Name: "read_file", Arguments: item.arguments}}},
			}}}}}
			var result ChatResult
			var err error
			if item.streaming {
				result, err = model.ChatWithStreamCtx(context.Background(), nil)
			} else {
				result, err = model.ChatCtx(context.Background(), nil)
			}
			if result.StopReason != item.stopReason {
				t.Fatalf("stop reason=%q want=%q", result.StopReason, item.stopReason)
			}
			if item.valid {
				if err != nil || len(result.ToolCalls) != 1 {
					t.Fatalf("valid call rejected: result=%+v err=%v", result, err)
				}
			} else if !errors.Is(err, ErrIncompleteToolCall) || len(result.ToolCalls) != 0 {
				t.Fatalf("incomplete call exposed: result=%+v err=%v", result, err)
			}
		})
	}
}

func TestInvalidLaterChoiceSuppressesAllTools(t *testing.T) {
	response := &llms.ContentResponse{Choices: []*llms.ContentChoice{
		{StopReason: "tool_calls", ToolCalls: []llms.ToolCall{{FunctionCall: &llms.FunctionCall{Name: "read_file", Arguments: `{}`}}}},
		{StopReason: "length", ToolCalls: []llms.ToolCall{{FunctionCall: &llms.FunctionCall{Name: "read_file", Arguments: `{}`}}}},
	}}
	result, err := resultFromResponse(response, false)
	if !errors.Is(err, ErrIncompleteToolCall) || len(result.ToolCalls) != 0 || result.StopReason != "length" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestStopReasonUsesProviderMetadataWithoutBreakingZeroValue(t *testing.T) {
	if (modelport.ChatResult{}).StopReason != "" {
		t.Fatal("zero result must have unknown stop reason")
	}
	for _, key := range []string{"StopReason", "FinishReason", "stop_reason", "finish_reason", "done_reason"} {
		t.Run(key, func(t *testing.T) {
			result, err := resultFromResponse(&llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: "ok", GenerationInfo: map[string]any{key: "end_turn"}}}}, false)
			if err != nil || result.StopReason != "end_turn" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestStreamingResponseCompletesOnlyMissingSuffix(t *testing.T) {
	var output strings.Builder
	model := &Model{LlmModel: functionLLM(func(ctx context.Context, _ []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
		if err := resolveOptions(options).StreamingFunc(ctx, []byte("hello")); err != nil {
			return nil, err
		}
		return responseWithText("hello world"), nil
	})}
	result, err := model.ChatWithStreamHandlerCtx(context.Background(), nil, ChatStreamHandler{OnAnswer: func(text string) { output.WriteString(text) }})
	if err != nil || result.Content != "hello world" || output.String() != "hello world" {
		t.Fatalf("result=%+v output=%q err=%v", result, output.String(), err)
	}
}

func TestStrictStreamStopReasonIsOptInForExistingModels(t *testing.T) {
	for _, strict := range []bool{false, true} {
		model := &Model{RequireStreamStopReason: strict, LlmModel: &capturingLLM{response: &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: "answer"}}}}}
		_, err := model.ChatWithStreamCtx(context.Background(), nil)
		if strict && !errors.Is(err, ErrIncompleteResponse) || !strict && err != nil {
			t.Fatalf("strict=%v err=%v", strict, err)
		}
	}
}

func TestModelInfoBudgetMetadataKeepsUnknownAndExplicitValues(t *testing.T) {
	var client Client
	client.SetModelInfo("unknown", nil, ModelInfo{Enabled: true})
	info, ok := client.GetModelInfo("unknown")
	if !ok || info.ContextWindowTokens != 0 || info.MaxOutputTokens != 0 {
		t.Fatalf("unknown budget must stay unknown: %+v", info)
	}
	client.SetModelInfo("custom", nil, ModelInfo{Enabled: true, ContextWindowTokens: 32768, MaxOutputTokens: 4096})
	info, ok = client.GetModelInfo("custom")
	if !ok || info.ContextWindowTokens != 32768 || info.MaxOutputTokens != 4096 {
		t.Fatalf("explicit metadata lost: %+v", info)
	}
}
