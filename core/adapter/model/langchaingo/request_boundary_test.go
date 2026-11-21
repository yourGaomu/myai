package langchaingo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"myai/core/contextmgr"
	compaction "myai/core/domain/compaction"
	"myai/core/domain/generation"
	domainmessage "myai/core/domain/message"
	domainmodel "myai/core/domain/model"
	corellm "myai/core/llm"
	modelport "myai/core/port/model"
)

func localOpenAIModel(t *testing.T, handler http.HandlerFunc) (*corellm.Model, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	model, err := (OpenAIChatCompletionsAdapter{}).CreateModel(modelport.CreationConfig{
		BaseURL: server.URL + "/v1", AuthType: domainmodel.AuthTypeNone, ModelName: "local-test-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	concrete := model.(*corellm.Model)
	concrete.Policy = corellm.RequestPolicy{RequestTimeout: 3 * time.Second, StreamIdleTimeout: time.Second, InitialBackoff: time.Millisecond, MaxBackoff: 2 * time.Second}
	return concrete, server
}

func localRequest() modelport.GenerateRequest {
	return modelport.GenerateRequest{
		Messages: []domainmessage.Message{domainmessage.Text(domainmessage.RoleUser, "hello")},
		Settings: generation.SystemDefaults(),
	}
}

func writeAnswer(w http.ResponseWriter, text, reason string) {
	w.Header().Set("Content-Type", "text/event-stream")
	frame := map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": text}, "finish_reason": reason}}}
	encoded, _ := json.Marshal(frame)
	fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", encoded)
}

func consumeRequest(r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
	r.Body.Close()
}

func TestChatCompletionsAdaptersSendDataURLOnUserMessage(t *testing.T) {
	picture := image.NewRGBA(image.Rect(0, 0, 7, 5))
	for _, tc := range []struct {
		adapter  modelport.ProtocolAdapter
		mimeType string
		encode   func(io.Writer) error
	}{
		{OpenAIChatCompletionsAdapter{}, "image/jpeg", func(w io.Writer) error { return jpeg.Encode(w, picture, nil) }},
		{MistralChatAdapter{}, "image/png", func(w io.Writer) error { return png.Encode(w, picture) }},
	} {
		t.Run(string(tc.adapter.Protocol()), func(t *testing.T) {
			var encoded bytes.Buffer
			if err := tc.encode(&encoded); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload struct {
					Messages []struct {
						Role    string          `json:"role"`
						Content json.RawMessage `json:"content"`
					} `json:"messages"`
				}
				if r.URL.Path != "/v1/chat/completions" || json.NewDecoder(r.Body).Decode(&payload) != nil || len(payload.Messages) != 3 {
					t.Error("invalid Chat Completions request")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				var text string
				if first := payload.Messages[0]; first.Role != "user" || json.Unmarshal(first.Content, &text) != nil || text != "plain text" {
					t.Error("text-only message shape changed")
				}
				var parts []struct {
					Type     string `json:"type"`
					Text     string `json:"text"`
					ImageURL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				}
				if user := payload.Messages[2]; user.Role != "user" || json.Unmarshal(user.Content, &parts) != nil || len(parts) != 2 || parts[0].Type != "text" || parts[0].Text != "see this" || parts[1].Type != "image_url" {
					t.Error("image missing from the target user content array")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				prefix, content, ok := strings.Cut(parts[1].ImageURL.URL, ",")
				data, err := base64.StdEncoding.DecodeString(content)
				decoded, _, decodeErr := image.Decode(bytes.NewReader(data))
				if !ok || prefix != "data:"+tc.mimeType+";base64" || err != nil || decodeErr != nil || !bytes.Equal(data, encoded.Bytes()) || decoded.Bounds().Dx() != 7 || decoded.Bounds().Dy() != 5 {
					t.Error("adapter did not send the original, decodable image")
				}
				if calls.Add(1) == 1 {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				writeAnswer(w, "ok", "stop")
			}))
			defer server.Close()
			model, err := tc.adapter.CreateModel(modelport.CreationConfig{BaseURL: server.URL + "/v1", ModelName: "local-fixture", APIKey: "unused-test-token"})
			if err != nil {
				t.Fatal(err)
			}
			if !model.(modelport.ImageInputSupport).SupportsImageInput() {
				t.Fatal("adapter did not opt into image input")
			}
			request := localRequest()
			request.Messages = []domainmessage.Message{
				domainmessage.Text(domainmessage.RoleUser, "plain text"), domainmessage.Assistant("answer", "reasoning", nil), domainmessage.Text(domainmessage.RoleUser, "see this"),
			}
			attachment := modelport.NewInputImage(tc.mimeType, encoded.Bytes())
			attachment.MessageIndex = 2
			request.Images = []modelport.InputImage{attachment}
			result, err := model.Generate(t.Context(), request)
			if err != nil || result.Content != "ok" || calls.Load() != 2 {
				t.Fatalf("result=%+v calls=%d err=%v", result, calls.Load(), err)
			}
		})
	}
}

func TestChatCompletionsImageRejectionIsExplicitWithoutProviderEcho(t *testing.T) {
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	request := localRequest()
	request.Images = []modelport.InputImage{modelport.NewInputImage("image/jpeg", encoded.Bytes())}
	for _, tc := range []struct {
		name string
		code int
		body string
		want error
	}{
		{"invalid", http.StatusBadRequest, `{"error":{"message":"echoed-private-data"}}`, modelport.ErrImageInputRejected},
		{"too large", http.StatusRequestEntityTooLarge, `{"error":{"message":"echoed-private-data"}}`, modelport.ErrImageInputRejected},
		{"context overflow", http.StatusBadRequest, `{"error":{"code":"context_length_exceeded","message":"echoed-private-data"}}`, modelport.ErrContextWindowExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			model, _ := localOpenAIModel(t, func(w http.ResponseWriter, r *http.Request) {
				consumeRequest(r)
				calls.Add(1)
				w.WriteHeader(tc.code)
				_, _ = io.WriteString(w, tc.body)
			})
			_, err := model.Generate(t.Context(), request)
			if !errors.Is(err, tc.want) || strings.Contains(err.Error(), "echoed-private-data") || calls.Load() != 1 {
				t.Fatalf("calls=%d error=%v", calls.Load(), err)
			}
		})
	}
}

func TestUnmappedImageProtocolRejectsRequestBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	model, err := (AnthropicMessagesAdapter{}).CreateModel(modelport.CreationConfig{BaseURL: server.URL, ModelName: "local", APIKey: "test-only"})
	if err != nil {
		t.Fatal(err)
	}
	request := localRequest()
	request.Images = []modelport.InputImage{modelport.NewInputImage("image/jpeg", nil)}
	if _, err := model.Generate(t.Context(), request); !errors.Is(err, modelport.ErrImageInputUnsupported) || calls.Load() != 0 {
		t.Fatalf("unsupported mapping reached HTTP or lost error: calls=%d error=%v", calls.Load(), err)
	}
}

func TestOpenAIRetriesTransientHTTPFailuresOnly(t *testing.T) {
	for _, status := range []int{429, 500, 503, 599, 400, 401, 403} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			model, _ := localOpenAIModel(t, func(w http.ResponseWriter, r *http.Request) {
				consumeRequest(r)
				if calls.Add(1) == 1 {
					w.WriteHeader(status)
					return
				}
				writeAnswer(w, "once", "stop")
			})
			var text strings.Builder
			request := localRequest()
			request.Stream.OnAnswer = func(chunk string) { text.WriteString(chunk) }
			result, err := model.Generate(context.Background(), request)
			if status == 429 || status >= 500 {
				if err != nil || calls.Load() != 2 || text.String() != "once" || result.StopReason != "stop" {
					t.Fatalf("calls=%d text=%q result=%+v err=%v", calls.Load(), text.String(), result, err)
				}
			} else {
				var requestError *corellm.RequestError
				if !errors.As(err, &requestError) || requestError.StatusCode != status || calls.Load() != 1 {
					t.Fatalf("calls=%d err=%v", calls.Load(), err)
				}
			}
		})
	}
}

func TestOpenAIHonorsRetryAfterAndRejectsInfeasibleWaits(t *testing.T) {
	for _, mode := range []string{"honor", "backoff cap", "remaining deadline"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			var first, second time.Time
			var mu sync.Mutex
			model, _ := localOpenAIModel(t, func(w http.ResponseWriter, r *http.Request) {
				consumeRequest(r)
				mu.Lock()
				defer mu.Unlock()
				if calls.Add(1) == 1 {
					first = time.Now()
					w.Header().Set("Retry-After", "1")
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}
				second = time.Now()
				writeAnswer(w, "ok", "stop")
			})
			if mode == "backoff cap" {
				model.Policy.MaxBackoff = 10 * time.Millisecond
			}
			if mode == "remaining deadline" {
				model.Policy.RequestTimeout = 50 * time.Millisecond
			}
			_, err := model.Generate(context.Background(), localRequest())
			mu.Lock()
			defer mu.Unlock()
			if mode == "honor" {
				if err != nil || calls.Load() != 2 || second.Sub(first) < time.Second {
					t.Fatalf("calls=%d delay=%v err=%v", calls.Load(), second.Sub(first), err)
				}
			} else if err == nil || calls.Load() != 1 {
				t.Fatalf("infeasible Retry-After retried: calls=%d err=%v", calls.Load(), err)
			}
		})
	}
}

func TestOpenAIRetriesNetworkFailureBeforeResponse(t *testing.T) {
	var calls atomic.Int32
	model, _ := localOpenAIModel(t, func(w http.ResponseWriter, r *http.Request) {
		consumeRequest(r)
		if calls.Add(1) == 1 {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			connection.Close()
			return
		}
		writeAnswer(w, "recovered", "stop")
	})
	result, err := model.Generate(context.Background(), localRequest())
	if err != nil || result.Content != "recovered" || calls.Load() != 2 {
		t.Fatalf("calls=%d result=%+v err=%v", calls.Load(), result, err)
	}
}

func TestOpenAINeverReplaysPartialStream(t *testing.T) {
	for _, failure := range []string{"broken body", "clean EOF without reason"} {
		t.Run(failure, func(t *testing.T) {
			var calls atomic.Int32
			model, _ := localOpenAIModel(t, func(w http.ResponseWriter, r *http.Request) {
				consumeRequest(r)
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				if failure == "broken body" {
					w.Header().Set("Content-Length", "10000")
				}
				fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n\n")
				w.(http.Flusher).Flush()
			})
			request := localRequest()
			var text strings.Builder
			request.Stream.OnAnswer = func(chunk string) { text.WriteString(chunk) }
			result, err := model.Generate(context.Background(), request)
			if err == nil || calls.Load() != 1 || text.String() != "partial" || len(result.ToolCalls) != 0 {
				t.Fatalf("calls=%d text=%q result=%+v err=%v", calls.Load(), text.String(), result, err)
			}
			if failure == "clean EOF without reason" && !errors.Is(err, corellm.ErrIncompleteResponse) {
				t.Fatalf("expected incomplete response, got %v", err)
			}
		})
	}
}

func TestOpenAICancellationDeadlineAndIdleReachHTTPServer(t *testing.T) {
	for _, mode := range []string{"cancel before headers", "cancel after chunk", "request deadline", "idle before first chunk", "idle after chunk"} {
		t.Run(mode, func(t *testing.T) {
			started := make(chan struct{})
			serverCanceled := make(chan struct{})
			model, _ := localOpenAIModel(t, func(w http.ResponseWriter, r *http.Request) {
				consumeRequest(r)
				close(started)
				if strings.Contains(mode, "after chunk") {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n\n")
					w.(http.Flusher).Flush()
				}
				<-r.Context().Done()
				close(serverCanceled)
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "request deadline" {
				model.Policy.RequestTimeout = 50 * time.Millisecond
			}
			if strings.HasPrefix(mode, "idle") {
				model.Policy.StreamIdleTimeout = 50 * time.Millisecond
			}
			request := localRequest()
			if mode == "cancel after chunk" {
				request.Stream.OnAnswer = func(string) { cancel() }
			}
			done := make(chan error, 1)
			go func() {
				_, err := model.Generate(ctx, request)
				done <- err
			}()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("HTTP request did not start")
			}
			if mode == "cancel before headers" {
				cancel()
			}
			var err error
			select {
			case err = <-done:
			case <-time.After(time.Second):
				t.Fatal("model did not stop")
			}
			expected := error(context.Canceled)
			if mode == "request deadline" {
				expected = context.DeadlineExceeded
			}
			if strings.HasPrefix(mode, "idle") {
				expected = corellm.ErrStreamIdleTimeout
			}
			if !errors.Is(err, expected) {
				t.Fatalf("err=%v want=%v", err, expected)
			}
			select {
			case <-serverCanceled:
			case <-time.After(time.Second):
				t.Fatal("cancellation did not reach HTTP server")
			}
		})
	}
}

func TestHTTPKeepalivesResetIdleWithoutAnswerCallbacks(t *testing.T) {
	model, _ := localOpenAIModel(t, func(w http.ResponseWriter, r *http.Request) {
		consumeRequest(r)
		w.Header().Set("Content-Type", "text/event-stream")
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; i < 6; i++ {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				fmt.Fprint(w, ": keepalive\n\n")
				w.(http.Flusher).Flush()
			}
		}
		writeAnswer(w, "done", "stop")
	})
	model.Policy.StreamIdleTimeout = 70 * time.Millisecond
	result, err := model.Generate(context.Background(), localRequest())
	if err != nil || result.Content != "done" {
		t.Fatalf("active stream failed: result=%+v err=%v", result, err)
	}
}

func TestOpenAIEmptyChoicesAndTruncatedToolArguments(t *testing.T) {
	for _, mode := range []string{"empty", "partial arguments", "valid JSON with length", "missing stop", "valid tool"} {
		t.Run(mode, func(t *testing.T) {
			model, _ := localOpenAIModel(t, func(w http.ResponseWriter, r *http.Request) {
				consumeRequest(r)
				w.Header().Set("Content-Type", "text/event-stream")
				if mode == "empty" {
					fmt.Fprint(w, "data: {\"choices\":[]}\n\ndata: [DONE]\n\n")
					return
				}
				arguments, reason := `{}`, "tool_calls"
				if mode == "partial arguments" {
					arguments = `{"path":`
				}
				if mode == "valid JSON with length" {
					reason = "length"
				}
				if mode == "missing stop" {
					reason = ""
				}
				frame := map[string]any{"choices": []any{map[string]any{"index": 0,
					"delta":         map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "call-1", "type": "function", "function": map[string]string{"name": "read_file", "arguments": arguments}}}},
					"finish_reason": reason,
				}}}
				encoded, _ := json.Marshal(frame)
				fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", encoded)
			})
			result, err := model.Generate(context.Background(), localRequest())
			switch mode {
			case "empty":
				if !errors.Is(err, corellm.ErrEmptyResponse) {
					t.Fatalf("expected empty response error, got %v", err)
				}
			case "valid tool":
				if err != nil || len(result.ToolCalls) != 1 || result.StopReason != "tool_calls" {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			default:
				if !errors.Is(err, corellm.ErrIncompleteToolCall) || len(result.ToolCalls) != 0 {
					t.Fatalf("unsafe tool response: result=%+v err=%v", result, err)
				}
			}
		})
	}
}

func TestMistralCompatibleClientPreservesPathMessagesToolsAndSettings(t *testing.T) {
	for _, suffix := range []string{"", "/v1"} {
		t.Run("base"+suffix, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/chat/completions" {
					t.Errorf("unexpected path %q", r.URL.Path)
				}
				var payload struct {
					Model    string `json:"model"`
					Messages []struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"messages"`
					Tools []struct {
						Type     string `json:"type"`
						Function struct {
							Name string `json:"name"`
						} `json:"function"`
					} `json:"tools"`
					MaxTokens   int     `json:"max_tokens"`
					Temperature float64 `json:"temperature"`
					TopP        float64 `json:"top_p"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if payload.Model != "mistral-local" || len(payload.Messages) != 1 || payload.Messages[0].Role != "user" || payload.Messages[0].Content != "hello" || len(payload.Tools) != 1 || payload.Tools[0].Function.Name != "read_file" || payload.MaxTokens != 128 || payload.Temperature != 0.2 || payload.TopP != 0.8 {
					t.Error("Mistral messages, tools, model or settings changed")
				}
				writeAnswer(w, "ok", "stop")
			}))
			defer server.Close()
			model, err := (MistralChatAdapter{}).CreateModel(modelport.CreationConfig{BaseURL: server.URL + suffix, ModelName: "mistral-local", AuthType: domainmodel.AuthTypeBearer, APIKey: "local-test"})
			if err != nil {
				t.Fatal(err)
			}
			request := localRequest()
			request.Settings = generation.ResolvedSettings{Temperature: 0.2, TopP: 0.8, MaxOutputTokens: 128}
			request.Tools = []modelport.Tool{{Type: "function", Function: &modelport.FunctionDefinition{Name: "read_file", Parameters: map[string]any{"type": "object"}}}}
			result, err := model.Generate(context.Background(), request)
			if err != nil || result.Content != "ok" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestMistralCompatibleClientCancellationReachesServer(t *testing.T) {
	serverCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		consumeRequest(r)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(serverCanceled)
	}))
	defer server.Close()
	model, err := (MistralChatAdapter{}).CreateModel(modelport.CreationConfig{BaseURL: server.URL, ModelName: "mistral-local", APIKey: "local-test"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := localRequest()
	request.Stream.OnAnswer = func(string) { cancel() }
	if _, err := model.Generate(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	select {
	case <-serverCanceled:
	case <-time.After(time.Second):
		t.Fatal("Mistral cancellation did not reach server")
	}
}

func TestStructuredContextOverflowIsDistinctFromOther400s(t *testing.T) {
	for _, item := range []struct {
		name     string
		body     string
		overflow bool
	}{
		{"known code", `{"error":{"code":"context_length_exceeded"}}`, true},
		{"known type", `{"error":{"type":"prompt_too_long"}}`, true},
		{"other invalid input", `{"error":{"code":"invalid_request_error"}}`, false},
		{"message alone", `{"error":{"message":"context length exceeded"}}`, false},
		{"plain text", `context_length_exceeded`, false},
	} {
		t.Run(item.name, func(t *testing.T) {
			var calls atomic.Int32
			model, _ := localOpenAIModel(t, func(w http.ResponseWriter, r *http.Request) {
				consumeRequest(r)
				calls.Add(1)
				w.WriteHeader(400)
				fmt.Fprint(w, item.body)
			})
			_, err := model.Generate(context.Background(), localRequest())
			if err == nil || modelport.IsContextWindowExceeded(err) != item.overflow || corellm.IsContextWindowExceeded(err) != item.overflow || calls.Load() != 1 {
				t.Fatalf("overflow=%v calls=%d err=%v", corellm.IsContextWindowExceeded(err), calls.Load(), err)
			}
		})
	}
}

func TestRetryAfterBackoffCancellationStopsBeforeSecondHTTPAttempt(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	model, _ := localOpenAIModel(t, func(w http.ResponseWriter, r *http.Request) {
		consumeRequest(r)
		calls.Add(1)
		close(started)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(429)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := model.Generate(ctx, localRequest())
		done <- err
	}()
	<-started
	timer := time.NewTimer(40 * time.Millisecond)
	defer timer.Stop()
	<-timer.C
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || calls.Load() != 1 {
			t.Fatalf("calls=%d err=%v", calls.Load(), err)
		}
	case <-time.After(time.Second):
		t.Fatal("Retry-After wait did not honor cancellation")
	}
}

func TestOllamaRetainsTerminalReasonAndDetectsMissingDone(t *testing.T) {
	for _, reason := range []string{"stop", "length", "missing"} {
		t.Run(reason, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				consumeRequest(r)
				if r.URL.Path != "/api/chat" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				fmt.Fprintln(w, `{"message":{"role":"assistant","content":"hello"},"done":false}`)
				if reason != "missing" {
					fmt.Fprintf(w, "{\"message\":{\"role\":\"assistant\",\"content\":\"\"},\"done\":true,\"done_reason\":%q}", reason)
				}
			}))
			defer server.Close()
			model, err := (OllamaChatAdapter{}).CreateModel(modelport.CreationConfig{BaseURL: server.URL, AuthType: domainmodel.AuthTypeNone, ModelName: "local-model"})
			if err != nil {
				t.Fatal(err)
			}
			result, err := model.Generate(context.Background(), localRequest())
			if reason == "missing" {
				if !errors.Is(err, corellm.ErrIncompleteResponse) {
					t.Fatalf("expected missing done error, got %v", err)
				}
			} else if err != nil || result.StopReason != reason || result.Content != "hello" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestContextSnapshotWirePreservesSystemAndRuntimeRoles(t *testing.T) {
	const fixedSystem = "Fixed assistant policy.\nPreserve project boundaries.\n固定系统策略完整保留。"
	const legacyRuntime = "obsolete runtime rules from a completed turn"
	const currentRuntime = "current turn runtime context"
	legacy := domainmessage.SyntheticText(domainmessage.SyntheticReasonRuntimeInstruction, legacyRuntime)
	current := domainmessage.RuntimeInstruction(currentRuntime)
	if legacy.Role != domainmessage.RoleSystem || current.Role != domainmessage.RoleUser {
		t.Fatal("fixture must include legacy system runtime and current user runtime")
	}
	options := contextmgr.RequestOptions{
		Budget:      contextmgr.NewRequestBudget(16000, 0, 1024),
		TurnContext: contextmgr.TurnContext{RuntimeInstruction: currentRuntime},
	}
	snapshot := contextmgr.BuildSnapshotForRequest([]domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, fixedSystem), legacy,
		domainmessage.Text(domainmessage.RoleUser, "earlier question"),
		domainmessage.Text(domainmessage.RoleAssistant, "earlier answer"),
		current, domainmessage.Text(domainmessage.RoleUser, "current question"),
	}, compaction.Checkpoint{}, 0, options)
	if err := options.Budget.ValidateSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	request := modelport.GenerateRequest{Messages: snapshot.Messages, Settings: generation.SystemDefaults()}
	request.Settings.MaxOutputTokens = 1024

	type wirePart struct {
		Text         string `json:"text"`
		CacheControl struct {
			Type string `json:"type"`
		} `json:"cache_control"`
		FunctionCall *struct {
			Name string            `json:"name"`
			Args map[string]string `json:"args"`
		} `json:"functionCall"`
		FunctionResponse *struct {
			Name     string            `json:"name"`
			Response map[string]string `json:"response"`
		} `json:"functionResponse"`
	}
	type wireMessage struct{ Role, Text string }
	wantConversation := []wireMessage{
		{"user", "earlier question"}, {"assistant", "earlier answer"},
		{"user", current.Text()}, {"user", "current question"},
	}
	for _, provider := range []string{"openai", "anthropic", "google"} {
		t.Run(provider, func(t *testing.T) {
			providerRequest := request
			wantHistory := wantConversation
			const toolName = "read_file"
			toolResult := domainmessage.ToolResult{ToolCallID: "call-local", Name: toolName, Content: "sample tool result"}
			if provider == "google" {
				providerRequest.Settings.Temperature = 0.25
				providerRequest.Settings.TopP = 0.75
				providerRequest.Tools = []modelport.Tool{{Type: "function", Function: &modelport.FunctionDefinition{
					Name: toolName, Description: "Read a file",
					Parameters: map[string]any{
						"type": "object", "required": []string{"path"},
						"properties": map[string]any{"path": map[string]any{"type": "string"}},
					},
				}}}
				providerRequest.Messages = slices.Insert(slices.Clone(request.Messages), len(request.Messages)-2,
					domainmessage.ToolCallMessage([]domainmessage.ToolCall{{ID: "call-local", Name: toolName, Arguments: `{"path":"note.txt"}`}}),
					domainmessage.ToolResultMessage(toolResult),
				)
				wantHistory = slices.Insert(slices.Clone(wantConversation), 2, wireMessage{"model", ""}, wireMessage{"user", ""})
				wantHistory[1].Role = "model"
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, err := io.ReadAll(r.Body)
				r.Body.Close()
				if err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				paths := map[string]string{"openai": "/v1/chat/completions", "anthropic": "/v1/messages", "google": "/v1beta/models/local-model:streamGenerateContent"}
				if r.Method != http.MethodPost || r.URL.Path != paths[provider] {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				if strings.Contains(string(body), legacyRuntime) {
					t.Error("legacy runtime survived projection into provider body")
				}
				var payload struct {
					System            json.RawMessage `json:"system"`
					SystemInstruction struct {
						Role  string     `json:"role"`
						Parts []wirePart `json:"parts"`
					} `json:"systemInstruction"`
					Messages []struct {
						Role    string          `json:"role"`
						Content json.RawMessage `json:"content"`
					} `json:"messages"`
					Contents []struct {
						Role  string     `json:"role"`
						Parts []wirePart `json:"parts"`
					} `json:"contents"`
					Tools []struct {
						FunctionDeclarations []struct {
							Name        string `json:"name"`
							Description string `json:"description"`
							Parameters  struct {
								Type       int      `json:"type"`
								Required   []string `json:"required"`
								Properties map[string]struct {
									Type int `json:"type"`
								} `json:"properties"`
							} `json:"parameters"`
						} `json:"functionDeclarations"`
					} `json:"tools"`
					GenerationConfig struct {
						MaxOutputTokens int     `json:"maxOutputTokens"`
						Temperature     float64 `json:"temperature"`
						TopP            float64 `json:"topP"`
						CandidateCount  int     `json:"candidateCount"`
					} `json:"generationConfig"`
				}
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				joinParts := func(parts []wirePart) string {
					var text strings.Builder
					for _, part := range parts {
						text.WriteString(part.Text)
					}
					return text.String()
				}
				policy := joinParts(payload.SystemInstruction.Parts)
				if provider == "anthropic" {
					var parts []wirePart
					if err := json.Unmarshal(payload.System, &parts); err != nil {
						t.Error(err)
					} else if len(parts) != 1 || parts[0].CacheControl.Type != "ephemeral" {
						t.Errorf("expected one cached Anthropic system block, got %+v", parts)
					}
					policy = joinParts(parts)
				}
				var conversation []wireMessage
				systems := 0
				for index, message := range payload.Messages {
					var text string
					if err := json.Unmarshal(message.Content, &text); err != nil {
						var parts []wirePart
						if err := json.Unmarshal(message.Content, &parts); err != nil {
							t.Error(err)
						}
						text = joinParts(parts)
					}
					if message.Role == "system" {
						systems++
						policy += text
						if index != 0 {
							t.Error("system message moved into conversation history")
						}
					} else {
						conversation = append(conversation, wireMessage{message.Role, text})
					}
				}
				if provider == "openai" && systems != 1 || provider != "openai" && systems != 0 {
					t.Errorf("unexpected system message count %d", systems)
				}
				for _, content := range payload.Contents {
					conversation = append(conversation, wireMessage{content.Role, joinParts(content.Parts)})
				}
				if policy != fixedSystem {
					t.Error("fixed system policy was truncated, replaced or combined with runtime context")
				}
				if !slices.Equal(conversation, wantHistory) {
					t.Errorf("conversation = %+v, want %+v", conversation, wantHistory)
				}
				switch provider {
				case "openai":
					writeAnswer(w, "ok", "stop")
				case "anthropic":
					w.Header().Set("Content-Type", "text/event-stream")
					for _, event := range []string{
						`{"type":"message_start","message":{"id":"local","type":"message","role":"assistant","model":"local-model","usage":{"input_tokens":1}}}`,
						`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
						`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`,
						`{"type":"content_block_stop","index":0}`,
						`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
						`{"type":"message_stop"}`,
					} {
						fmt.Fprintf(w, "data: %s\n\n", event)
					}
				case "google":
					if payload.SystemInstruction.Role != "system" {
						t.Errorf("Gemini systemInstruction role=%q, want system", payload.SystemInstruction.Role)
					}
					if query := r.URL.Query(); query.Get("key") != "local-test" || query.Get("$alt") != "json;enum-encoding=int" || query.Get("alt") == "sse" {
						t.Error("Gemini request did not use API key auth and native JSON streaming")
					}
					if config := payload.GenerationConfig; config.MaxOutputTokens != 1024 || config.Temperature != 0.25 || config.TopP != 0.75 || config.CandidateCount != 1 {
						t.Errorf("unexpected Gemini generation settings: %+v", config)
					}
					const schemaObject, schemaString = 6, 1 // Gemini's enum-encoding=int wire values.
					if len(payload.Tools) != 1 || len(payload.Tools[0].FunctionDeclarations) != 1 {
						t.Errorf("unexpected Gemini tools: %+v", payload.Tools)
					} else if tool := payload.Tools[0].FunctionDeclarations[0]; tool.Name != toolName || tool.Description != "Read a file" || tool.Parameters.Type != schemaObject || tool.Parameters.Properties["path"].Type != schemaString || !slices.Equal(tool.Parameters.Required, []string{"path"}) {
						t.Errorf("unexpected Gemini function declaration: %+v", tool)
					}
					if len(payload.Contents) >= 4 {
						if parts := payload.Contents[2].Parts; len(parts) != 1 || parts[0].FunctionCall == nil || parts[0].FunctionCall.Name != toolName || parts[0].FunctionCall.Args["path"] != "note.txt" {
							t.Errorf("unexpected Gemini function call history: %+v", parts)
						}
						if parts := payload.Contents[3].Parts; len(parts) != 1 || parts[0].FunctionResponse == nil || parts[0].FunctionResponse.Name != toolName || parts[0].FunctionResponse.Response["response"] != toolResult.PromptContent() {
							t.Errorf("unexpected Gemini function response history: %+v", parts)
						}
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `[{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}]`)
				}
			}))
			defer server.Close()
			var model modelport.ChatModelPort
			var err error
			switch provider {
			case "openai":
				model, err = (OpenAIChatCompletionsAdapter{}).CreateModel(modelport.CreationConfig{BaseURL: server.URL + "/v1", ModelName: "local-model", APIKey: "local-test"})
			case "anthropic":
				model, err = (AnthropicMessagesAdapter{}).CreateModel(modelport.CreationConfig{BaseURL: server.URL + "/v1", ModelName: "local-model", APIKey: "local-test"})
			case "google":
				model, err = createGoogleGenerativeAIModel(modelport.CreationConfig{
					Protocol: domainmodel.ProtocolGoogleGenerativeAI, AuthType: domainmodel.AuthTypeBearer,
					ModelName: "local-model", APIKey: "local-test",
				}, server.Client().Transport, server.URL)
			}
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			result, err := model.Generate(ctx, providerRequest)
			if err != nil || result.Content != "ok" || calls.Load() != 1 {
				t.Fatalf("calls=%d content=%q err=%v", calls.Load(), result.Content, err)
			}
		})
	}
}
