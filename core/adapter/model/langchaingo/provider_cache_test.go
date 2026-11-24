package langchaingo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tmc/langchaingo/llms"

	domainmessage "myai/core/domain/message"
	corellm "myai/core/llm"
	modelport "myai/core/port/model"
)

func TestOpenAIUsageWirePresence(t *testing.T) {
	const totals = `"prompt_tokens":20,"completion_tokens":4,"total_tokens":24`
	for _, tc := range []struct {
		name, fields string
		count        int
		reported     bool
	}{
		{"standard preferred", `"prompt_tokens_details":{"cached_tokens":15},"prompt_cache_hit_tokens":11`, 15, true},
		{"deepseek", `"prompt_cache_hit_tokens":12,"prompt_cache_miss_tokens":8`, 12, true},
		{"input details", `"input_tokens_details":{"cached_tokens":13}`, 13, true},
		{"flat alias", `"cached_prompt_tokens":14`, 14, true},
		{"standard zero overrides alternate", `"prompt_tokens_details":{"cached_tokens":0},"input_tokens_details":{"cached_tokens":13}`, 0, true},
		{"deepseek zero", `"prompt_cache_hit_tokens":0`, 0, true},
		{"omitted", `"prompt_tokens_details":{},"prompt_cache_miss_tokens":20`, 0, false},
		{"null or invalid", `"prompt_tokens_details":{"cached_tokens":null},"prompt_cache_hit_tokens":-4,"cached_tokens":"0"`, 0, false},
		{"no usage", "", 0, false},
	} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, streaming), func(t *testing.T) {
				var usage string
				if tc.fields != "" {
					usage = `,"usage":{` + totals + "," + tc.fields + "}"
				}
				model, _ := localOpenAIModel(t, func(w http.ResponseWriter, r *http.Request) {
					var payload struct {
						Stream        bool            `json:"stream"`
						CacheControl  json.RawMessage `json:"cache_control"`
						StreamOptions struct {
							IncludeUsage bool `json:"include_usage"`
						} `json:"stream_options"`
					}
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Stream != streaming || payload.StreamOptions.IncludeUsage != streaming || payload.CacheControl != nil {
						t.Errorf("unexpected request: %+v, err=%v", payload, err)
					}
					if !streaming {
						w.Header().Set("Content-Type", "application/json")
						fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]`+usage+`}`)
						return
					}
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, ": keepalive\r\n\r\n"+`data: {"choices":[{"index":0,"delta":{"content":"ok","reasoning_content":"think"}}],"usage":null}`+"\r\n\r\n")
					fmt.Fprint(w, `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n")
					if usage != "" {
						fmt.Fprint(w, `data:{"choices":[]`+usage+"}\r\n\r\n")
						// The later usage frame must not erase cache counts.
						fmt.Fprint(w, `data: {"choices":[],"usage":{`+totals+`}}`+"\n\n")
					}
					fmt.Fprint(w, "data: [DONE]\n\n")
				})
				var result modelport.ChatResult
				var err error
				if streaming {
					request := localRequest()
					var answer, reasoning strings.Builder
					request.Stream.OnAnswer = func(s string) { answer.WriteString(s) }
					request.Stream.OnReasoning = func(s string) { reasoning.WriteString(s) }
					result, err = model.Generate(context.Background(), request)
					if answer.String() != "ok" || reasoning.String() != "think" || result.Reasoning != "think" {
						t.Errorf("answer=%q reasoning callback=%q result reasoning=%q", answer.String(), reasoning.String(), result.Reasoning)
					}
				} else {
					result, err = model.Chat([]llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "hello")})
				}
				if err != nil || result.Content != "ok" || result.StopReason != "stop" {
					t.Fatalf("result=%+v, err=%v", result, err)
				}
				want := modelport.TokenUsage{PromptCachedTokens: tc.count, PromptCacheReported: tc.reported, PromptCacheIncomplete: !tc.reported}
				if tc.fields != "" {
					want.Available, want.PromptTokens, want.CompletionTokens, want.TotalTokens = true, 20, 4, 24
				}
				if result.Usage != want {
					t.Fatalf("usage=%+v, want %+v", result.Usage, want)
				}
			})
		}
	}
}

func TestOpenAIUsageAfterLargeNonstreamContent(t *testing.T) {
	content := strings.Repeat(`content with { "usage": [ ] }, \`, 1+(maxUsageBytes/16))
	model, _ := localOpenAIModel(t, func(w http.ResponseWriter, r *http.Request) {
		consumeRequest(r)
		encoded, _ := json.Marshal(content)
		fmt.Fprintf(w, `{"unrelated":{"usage":{"cached_tokens":99}},"choices":[{"message":{"content":%s},"finish_reason":"stop"}],"\u0075sage":{"prompt_tokens":17,"completion_tokens":8,"cached_tokens":11}}`, encoded)
	})
	result, err := model.Chat([]llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "hello")})
	if err != nil || result.Content != content || result.Usage.TotalTokens != 25 || result.Usage.PromptCachedTokens != 11 || !result.Usage.PromptCacheReported {
		t.Fatalf("content length=%d usage=%+v err=%v", len(result.Content), result.Usage, err)
	}
}

func TestOpenAIUsageObserverDoesNotWaitToDeliverAnswer(t *testing.T) {
	answer := make(chan struct{})
	model, _ := localOpenAIModel(t, func(w http.ResponseWriter, r *http.Request) {
		consumeRequest(r)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"index":0,"delta":{"content":"ok"}}]}`+"\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-answer:
		case <-r.Context().Done():
			t.Error("answer was not delivered before the terminal usage frame")
			return
		}
		fmt.Fprint(w, `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n")
		for _, part := range []string{`da`, `ta: {"choices":[],"usa`, `ge":{"prompt_tokens":5,"completion_tokens":3,"prompt_cache_hit_tokens":4}}`, "\n\n"} {
			fmt.Fprint(w, part)
			w.(http.Flusher).Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	request := localRequest()
	request.Stream.OnAnswer = func(s string) {
		if s == "ok" {
			close(answer)
		}
	}
	result, err := model.Generate(context.Background(), request)
	if err != nil || result.Usage.TotalTokens != 8 || result.Usage.PromptCachedTokens != 4 || !result.Usage.PromptCacheReported {
		t.Fatalf("result=%+v, err=%v", result, err)
	}
}

func newLocalAnthropic(t *testing.T, handler http.HandlerFunc) *corellm.Model {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	model, err := (AnthropicMessagesAdapter{}).CreateModel(modelport.CreationConfig{BaseURL: server.URL + "/v1", ModelName: "local", APIKey: "test-only"})
	if err != nil {
		t.Fatal(err)
	}
	concrete := model.(*corellm.Model)
	concrete.Policy = corellm.RequestPolicy{RequestTimeout: 3 * time.Second, StreamIdleTimeout: time.Second}
	return concrete
}

func writeAnthropicAnswer(w http.ResponseWriter, inputUsage string, streaming bool) {
	if !streaming {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"local","model":"local","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{%s,"output_tokens":7}}`, inputUsage)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	for _, event := range []string{
		`{"type":"message_start","message":{"id":"local","type":"message","role":"assistant","model":"local","usage":{` + inputUsage + `,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`,
		`{"type":"message_stop"}`,
	} {
		fmt.Fprintf(w, "data: %s\n\n", event)
	}
}

func TestAnthropicUsageIncludesAllInputComponents(t *testing.T) {
	for _, tc := range []struct {
		name, fields   string
		prompt, cached int
		reported       bool
	}{
		{"hits and writes", `"input_tokens":9,"cache_creation_input_tokens":12,"cache_read_input_tokens":30`, 51, 30, true},
		{"writes and explicit zero hits", `"input_tokens":9,"cache_creation_input_tokens":12,"cache_read_input_tokens":0`, 21, 0, true},
		{"writes without reported hits", `"input_tokens":9,"cache_creation_input_tokens":12`, 21, 0, false},
		{"no cache fields", `"input_tokens":9`, 9, 0, false},
	} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, streaming), func(t *testing.T) {
				model := newLocalAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
					consumeRequest(r)
					writeAnthropicAnswer(w, tc.fields, streaming)
				})
				var result modelport.ChatResult
				var err error
				if streaming {
					result, err = model.Generate(context.Background(), localRequest())
				} else {
					result, err = model.Chat([]llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "hello")})
				}
				want := modelport.TokenUsage{PromptTokens: tc.prompt, CompletionTokens: 7, TotalTokens: tc.prompt + 7, Available: true, PromptCachedTokens: tc.cached, PromptCacheReported: tc.reported, PromptCacheIncomplete: !tc.reported}
				if err != nil || result.Content != "ok" || result.Usage != want {
					t.Fatalf("result=%+v, want usage %+v, err=%v", result, want, err)
				}
			})
		}
	}
}

func TestAnthropicCacheBreakpointsAdvanceThroughCompletedBlocks(t *testing.T) {
	type block struct {
		Text  string `json:"text"`
		Type  string `json:"type"`
		Cache struct {
			Type string `json:"type"`
		} `json:"cache_control"`
	}
	type body struct {
		System   []block `json:"system"`
		Tools    []block `json:"tools"`
		Messages []struct {
			Content []block `json:"content"`
		} `json:"messages"`
	}
	bodies := make(chan body, 4)
	model := newLocalAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
		var payload body
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode Anthropic body: %v", err)
		}
		bodies <- payload
		writeAnthropicAnswer(w, `"input_tokens":9`, true)
	})
	fixed := domainmessage.Text(domainmessage.RoleSystem, "fixed")
	ask := func(s string) domainmessage.Message { return domainmessage.Text(domainmessage.RoleUser, s) }
	answer := func(s string) domainmessage.Message { return domainmessage.Text(domainmessage.RoleAssistant, s) }
	tools := []modelport.Tool{{Type: "function", Function: &modelport.FunctionDefinition{Name: "local_tool", Parameters: map[string]any{"type": "object"}}}}
	send := func(messages ...domainmessage.Message) body {
		request := localRequest()
		request.Messages, request.Tools = messages, tools
		if _, err := model.Generate(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		return <-bodies
	}
	call := domainmessage.ToolCallMessage([]domainmessage.ToolCall{{ID: "toolu_local", Name: "local_tool", Arguments: "{}"}})
	result := domainmessage.ToolResultMessage(domainmessage.ToolResult{ToolCallID: "toolu_local", Name: "local_tool", Content: "tool value"})
	first := send(fixed, ask("one"), answer("first anchor"), ask("two"), call, result)
	second := send(fixed, ask("one"), answer("first anchor"), ask("two"), call, result, domainmessage.EnvironmentContext("volatile"), ask("three"))
	third := send(fixed, domainmessage.SyntheticText(domainmessage.SyntheticReasonPlanContext, "volatile system"), ask("one"), answer("first anchor"), ask("two"))
	fallback := send(ask("one"))
	for index, payload := range []body{first, second, third} {
		if len(payload.System) == 0 || payload.System[0].Type != "text" || payload.System[0].Text != "fixed" || payload.System[0].Cache.Type != "ephemeral" || len(payload.Tools) != 1 || payload.Tools[0].Cache.Type != "" {
			t.Fatalf("request %d fixed/tool breakpoint: %+v", index, payload)
		}
	}
	if len(third.System) != 2 || third.System[1].Text != "volatile system" || third.System[1].Cache.Type != "" {
		t.Fatalf("marked or changed volatile system: %+v", third.System)
	}
	if len(fallback.System) != 0 || len(fallback.Tools) != 1 || fallback.Tools[0].Cache.Type != "ephemeral" {
		t.Fatalf("no fixed tool fallback: %+v", fallback)
	}
	for i, tc := range []struct {
		body body
		want []int
	}{{first, []int{1, 4}}, {second, []int{1, 4}}, {third, nil}, {fallback, nil}} {
		var found []int
		for j, message := range tc.body.Messages {
			for _, part := range message.Content {
				if part.Cache.Type == "ephemeral" {
					found = append(found, j)
				}
			}
		}
		if !reflect.DeepEqual(found, tc.want) {
			t.Fatalf("request %d history cache indices=%v, want %v", i, found, tc.want)
		}
	}
}
