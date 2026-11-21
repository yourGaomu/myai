package langchaingo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tmc/langchaingo/llms"

	domainmodel "myai/core/domain/model"
	corellm "myai/core/llm"
	modelport "myai/core/port/model"
)

const (
	geminiFixtureModel = "local-gemini"
	geminiStreamPath   = "/v1beta/models/" + geminiFixtureModel + ":streamGenerateContent"
	geminiUnaryPath    = "/v1beta/models/" + geminiFixtureModel + ":generateContent"
	geminiAnswerFrame  = `{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"answer"}]}}]}`
	geminiStopFrame    = `{"candidates":[{"index":0,"finishReason":"STOP"}]}`
	geminiToolFrame    = `{"candidates":[{"index":0,"content":{"role":"model","parts":[{"functionCall":{"name":"read_file","args":{"path":"note.txt"}}}]}}]}`
)

func localGeminiModel(t *testing.T, path string, respond http.HandlerFunc) *corellm.Model {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != path {
			t.Errorf("Gemini HTTP request = %s %s, want POST %s", r.Method, r.URL.Path, path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		respond(w, r)
	}))
	t.Cleanup(server.Close)
	model, err := createGoogleGenerativeAIModel(modelport.CreationConfig{
		Protocol: domainmodel.ProtocolGoogleGenerativeAI, AuthType: domainmodel.AuthTypeBearer,
		ModelName: geminiFixtureModel, APIKey: "local-test",
	}, server.Client().Transport, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	model.Policy = corellm.RequestPolicy{
		RequestTimeout: 5 * time.Second, StreamIdleTimeout: 3 * time.Second,
		InitialBackoff: time.Millisecond, MaxAttempts: 3,
	}
	return model
}

func TestGeminiStreamDeliversAnswerBeforeFinalUsage(t *testing.T) {
	answerReceived := make(chan struct{})
	model := localGeminiModel(t, geminiStreamPath, func(w http.ResponseWriter, r *http.Request) {
		consumeRequest(r)
		io.WriteString(w, `[{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"private thought","thought":true},{"text":"hello "}]}}],"usageMetadata":{"promptTokenCount":18,"candidatesTokenCount":1}}`+",\n")
		w.(http.Flusher).Flush()
		select {
		case <-answerReceived:
		case <-r.Context().Done():
			t.Error("Gemini withheld the answer until the final usage frame")
			return
		}
		io.WriteString(w, `{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"world"}]}}]},`+geminiStopFrame+`,
{"usageMetadata":{"promptTokenCount":20,"cachedContentTokenCount":12,"thoughtsTokenCount":2}},
{"candidates":[],"usageMetadata":{"candidatesTokenCount":4,"totalTokenCount":26}}]`)
	})
	request := localRequest()
	var chunks []string
	request.Stream.OnAnswer = func(chunk string) {
		chunks = append(chunks, chunk)
		if len(chunks) == 1 && chunk == "hello " {
			close(answerReceived)
		}
	}
	result, err := model.Generate(t.Context(), request)
	wantUsage := modelport.TokenUsage{PromptTokens: 20, CompletionTokens: 4, TotalTokens: 26, ReasoningTokens: 2, Available: true, PromptCachedTokens: 12, PromptCacheReported: true}
	if err != nil || result.Content != "hello world" || result.StopReason != "FinishReasonStop" || result.Usage != wantUsage {
		t.Fatalf("result=%+v, want usage %+v, err=%v", result, wantUsage, err)
	}
	if !slices.Equal(chunks, []string{"hello ", "world"}) {
		t.Fatalf("answer chunks=%q, want each visible text frame exactly once", chunks)
	}
}

func TestGeminiStreamUsagePresence(t *testing.T) {
	for _, tc := range []struct {
		name, frames string
		want         modelport.TokenUsage
	}{
		{"explicit zero overrides earlier count", `,{"usageMetadata":{"cachedContentTokenCount":5}},{"usageMetadata":{"cachedContentTokenCount":0}},{"usageMetadata":{"promptTokenCount":6,"candidatesTokenCount":2}}`,
			modelport.TokenUsage{Available: true, PromptTokens: 6, CompletionTokens: 2, TotalTokens: 8, PromptCacheReported: true}},
		{"omitted cache count", `,{"usageMetadata":{"promptTokenCount":6,"candidatesTokenCount":2,"totalTokenCount":8}}`,
			modelport.TokenUsage{Available: true, PromptTokens: 6, CompletionTokens: 2, TotalTokens: 8, PromptCacheIncomplete: true}},
		{"no usage metadata", "", modelport.TokenUsage{PromptCacheIncomplete: true}},
		{"only prompt count", `,{"usageMetadata":{"promptTokenCount":6}}`,
			modelport.TokenUsage{Available: true, PromptTokens: 6, PromptCacheIncomplete: true}},
		{"actual zero token counts", `,{"usageMetadata":{"promptTokenCount":0,"candidatesTokenCount":0,"totalTokenCount":0,"cachedContentTokenCount":0}}`,
			modelport.TokenUsage{Available: true, PromptCacheReported: true}},
		{"invalid counts do not overwrite valid counts", `,{"usageMetadata":{"promptTokenCount":6,"cachedContentTokenCount":4}},{"usageMetadata":{"promptTokenCount":"0","cachedContentTokenCount":-1,"candidatesTokenCount":2}},{"usageMetadata":{"cachedContentTokenCount":null}}`,
			modelport.TokenUsage{Available: true, PromptTokens: 6, CompletionTokens: 2, TotalTokens: 8, PromptCachedTokens: 4, PromptCacheReported: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := localGeminiModel(t, geminiStreamPath, func(w http.ResponseWriter, r *http.Request) {
				consumeRequest(r)
				io.WriteString(w, "["+geminiAnswerFrame+","+geminiStopFrame+tc.frames+"]")
			})
			result, err := model.Generate(t.Context(), localRequest())
			if err != nil || result.Content != "answer" || result.Usage != tc.want {
				t.Fatalf("result=%+v, want usage %+v, err=%v", result, tc.want, err)
			}
		})
	}
}

func TestGeminiChatCtxPreservesUnaryAndHistoryWirePaths(t *testing.T) {
	human := func(text string) llms.MessageContent { return llms.TextParts(llms.ChatMessageTypeHuman, text) }
	for _, tc := range []struct {
		name, path, response string
		messages             []llms.MessageContent
		want                 modelport.TokenUsage
	}{
		{"single user uses unary", geminiUnaryPath,
			`{"candidates":[{"content":{"role":"model","parts":[{"text":"answer"}]},"finishReason":1}],"usageMetadata":{"promptTokenCount":9,"cachedContentTokenCount":0,"candidatesTokenCount":3,"totalTokenCount":12}}`,
			[]llms.MessageContent{human("current question")},
			modelport.TokenUsage{Available: true, PromptTokens: 9, CompletionTokens: 3, TotalTokens: 12, PromptCacheReported: true}},
		{"history streams even without callbacks", geminiStreamPath,
			"[" + geminiAnswerFrame + "," + geminiStopFrame + `,{"usageMetadata":{"promptTokenCount":20,"cachedContentTokenCount":11,"candidatesTokenCount":3,"totalTokenCount":23}}]`,
			[]llms.MessageContent{human("earlier question"), llms.TextParts(llms.ChatMessageTypeAI, "earlier answer"), human("current question")},
			modelport.TokenUsage{Available: true, PromptTokens: 20, CompletionTokens: 3, TotalTokens: 23, PromptCachedTokens: 11, PromptCacheReported: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := localGeminiModel(t, tc.path, func(w http.ResponseWriter, r *http.Request) {
				consumeRequest(r)
				io.WriteString(w, tc.response)
			})
			result, err := model.ChatCtx(t.Context(), tc.messages)
			if err != nil || result.Content != "answer" || result.StopReason != "FinishReasonStop" || result.Usage != tc.want {
				t.Fatalf("result=%+v, want usage %+v, err=%v", result, tc.want, err)
			}
		})
	}
}

func TestGeminiStreamRejectsInvalidOrUnsafeToolsWithoutReplay(t *testing.T) {
	for _, tc := range []struct {
		name, body, visible string
		wantErr             error
	}{
		{"malformed after stop", "[" + geminiAnswerFrame + "," + geminiToolFrame + "," + geminiStopFrame + `,{"usageMetadata":}]`, "answer", nil},
		{"truncated after stop", "[" + geminiAnswerFrame + "," + geminiToolFrame + "," + geminiStopFrame, "answer", nil},
		{"truncated function arguments", "[" + geminiAnswerFrame + `,{"candidates":[{"content":{"parts":[{"functionCall":{"name":"read_file","args":{"path":`, "answer", nil},
		{"usage cannot replace stop", "[" + geminiAnswerFrame + `,{"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":3}}]`, "answer", corellm.ErrIncompleteResponse},
		{"prompt blocked", `[{"promptFeedback":{"blockReason":"SAFETY"}}]`, "", nil},
		{"candidate blocked after tool", "[" + geminiToolFrame + `,{"candidates":[{"index":0,"finishReason":3}]}]`, "", nil},
		{"recitation after tool", "[" + geminiToolFrame + `,{"candidates":[{"index":0,"finishReason":"RECITATION"}]}]`, "", nil},
		{"no terminal reason for tool", "[" + geminiToolFrame + "]", "", corellm.ErrIncompleteToolCall},
		{"max tokens despite valid tool args", "[" + geminiToolFrame + `,{"candidates":[{"index":0,"finishReason":"MAX_TOKENS"}]}]`, "", corellm.ErrIncompleteToolCall},
		{"arguments must be an object", `[{"candidates":[{"content":{"parts":[{"functionCall":{"name":"read_file","args":[]}}]},"finishReason":"STOP"}]}]`, "", corellm.ErrIncompleteToolCall},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			model := localGeminiModel(t, geminiStreamPath, func(w http.ResponseWriter, r *http.Request) {
				consumeRequest(r)
				calls.Add(1)
				io.WriteString(w, tc.body)
			})
			request := localRequest()
			var answer strings.Builder
			request.Stream.OnAnswer = func(chunk string) { answer.WriteString(chunk) }
			result, err := model.Generate(t.Context(), request)
			if err == nil || tc.wantErr != nil && !errors.Is(err, tc.wantErr) || len(result.ToolCalls) != 0 || answer.String() != tc.visible || calls.Load() != 1 {
				t.Fatalf("calls=%d answer=%q result=%+v err=%v, want answer %q and failure %v without tools", calls.Load(), answer.String(), result, err, tc.visible, tc.wantErr)
			}
		})
	}
}

func TestGeminiStreamCommitsToolAfterCompleteResponse(t *testing.T) {
	model := localGeminiModel(t, geminiStreamPath, func(w http.ResponseWriter, r *http.Request) {
		consumeRequest(r)
		io.WriteString(w, "["+geminiToolFrame+","+geminiStopFrame+`,{"usageMetadata":{"promptTokenCount":8,"cachedContentTokenCount":5}}]`)
	})
	request := localRequest()
	request.Stream.OnAnswer = func(chunk string) { t.Errorf("tool-only response emitted visible text %q", chunk) }
	result, err := model.Generate(t.Context(), request)
	if err != nil || result.Content != "" || result.StopReason != "FinishReasonStop" || len(result.ToolCalls) != 1 || result.Usage.PromptCachedTokens != 5 || !result.Usage.PromptCacheReported {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	call := result.ToolCalls[0]
	var args map[string]string
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil || call.Name != "read_file" || len(args) != 1 || args["path"] != "note.txt" {
		t.Fatalf("unexpected Gemini tool call: %+v (args decode error: %v)", call, err)
	}
}

func TestGeminiStreamCancellationWithholdsPendingToolAndReachesServer(t *testing.T) {
	serverCanceled := make(chan struct{}, 1)
	var calls atomic.Int32
	model := localGeminiModel(t, geminiStreamPath, func(w http.ResponseWriter, r *http.Request) {
		consumeRequest(r)
		calls.Add(1)
		io.WriteString(w, "["+geminiToolFrame+","+geminiAnswerFrame+",\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		serverCanceled <- struct{}{}
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	request := localRequest()
	var answer strings.Builder
	request.Stream.OnAnswer = func(chunk string) {
		answer.WriteString(chunk)
		cancel()
	}
	result, err := model.Generate(ctx, request)
	if !errors.Is(err, context.Canceled) || calls.Load() != 1 || answer.String() != "answer" || len(result.ToolCalls) != 0 {
		t.Fatalf("calls=%d answer=%q result=%+v err=%v", calls.Load(), answer.String(), result, err)
	}
	select {
	case <-serverCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("Gemini cancellation did not reach the HTTP server")
	}
}
