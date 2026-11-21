package llm

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tmc/langchaingo/llms"
	"google.golang.org/api/googleapi"
)

type functionLLM func(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error)

func (f functionLLM) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	return f(ctx, messages, options...)
}

func (f functionLLM) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", errors.New("unexpected Call")
}

func responseWithText(text string) *llms.ContentResponse {
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: text, StopReason: "stop"}}}
}

func resolveOptions(options []llms.CallOption) llms.CallOptions {
	var call llms.CallOptions
	for _, option := range options {
		option(&call)
	}
	return call
}

func TestRequestPolicyZeroValueKeepsBoundedDefaults(t *testing.T) {
	policy := (RequestPolicy{}).resolved()
	if policy.RequestTimeout <= 0 || policy.StreamIdleTimeout <= 0 || policy.MaxAttempts != 3 || policy.MaxBackoff <= 0 {
		t.Fatalf("unbounded default policy: %+v", policy)
	}
	model := &Model{LlmModel: functionLLM(func(ctx context.Context, _ []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > DefaultRequestTimeout {
			t.Error("zero-value policy did not set a deadline")
		}
		return responseWithText("ok"), nil
	})}
	if _, err := model.ChatCtx(nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestObservedUsageDoesNotLeakAcrossAttempts(t *testing.T) {
	calls := 0
	model := &Model{
		Policy: RequestPolicy{MaxAttempts: 2, InitialBackoff: time.Millisecond},
		LlmModel: functionLLM(func(ctx context.Context, _ []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
			calls++
			if calls == 1 {
				ObserveTokenUsage(ctx, TokenUsage{Available: true, PromptTokens: 3, PromptCachedTokens: 2, PromptCacheReported: true})
				return nil, &RequestError{StatusCode: http.StatusServiceUnavailable}
			}
			return responseWithText("ok"), nil
		}),
	}
	result, err := model.Chat(nil)
	if err != nil || calls != 2 || result.Usage != (TokenUsage{PromptCacheIncomplete: true}) {
		t.Fatalf("calls=%d usage=%+v err=%v", calls, result.Usage, err)
	}
}

func TestModelRequestDeadlineAndCallerCancellation(t *testing.T) {
	for _, mode := range []string{"policy deadline", "caller deadline", "caller cancellation"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			cancel := func() {}
			policy := RequestPolicy{RequestTimeout: 40 * time.Millisecond, StreamIdleTimeout: time.Second}
			expected := error(context.DeadlineExceeded)
			if mode == "caller deadline" {
				ctx, cancel = context.WithTimeout(ctx, 20*time.Millisecond)
				policy.RequestTimeout = time.Second
			}
			if mode == "caller cancellation" {
				ctx, cancel = context.WithCancel(ctx)
				expected = context.Canceled
			}
			defer cancel()
			calls := 0
			model := &Model{Policy: policy, LlmModel: functionLLM(func(ctx context.Context, _ []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
				calls++
				if mode == "caller cancellation" {
					cancel()
				}
				<-ctx.Done()
				// Some SDKs swallow context errors; the boundary must restore them.
				return responseWithText("ignored"), nil
			})}
			_, err := model.ChatWithStreamCtx(ctx, nil)
			if !errors.Is(err, expected) || calls != 1 {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
		})
	}
}

func TestStreamIdleTimeoutCancelsSDKWithoutRetry(t *testing.T) {
	for _, initialChunk := range []bool{false, true} {
		t.Run(fmt.Sprint(initialChunk), func(t *testing.T) {
			calls := 0
			model := &Model{Policy: RequestPolicy{RequestTimeout: time.Second, StreamIdleTimeout: 25 * time.Millisecond}, LlmModel: functionLLM(func(ctx context.Context, _ []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
				calls++
				if initialChunk {
					if err := resolveOptions(options).StreamingFunc(ctx, []byte("partial")); err != nil {
						return nil, err
					}
				}
				<-ctx.Done()
				return responseWithText("ignored"), nil
			})}
			result, err := model.ChatWithStreamCtx(context.Background(), nil)
			if !errors.Is(err, ErrStreamIdleTimeout) || !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
			if initialChunk && result.Content != "partial" {
				t.Fatalf("partial content was lost: %+v", result)
			}
		})
	}
}

func TestStreamActivityResetsOnlyIdleDeadline(t *testing.T) {
	for _, fixedDeadline := range []bool{false, true} {
		t.Run(fmt.Sprint(fixedDeadline), func(t *testing.T) {
			policy := RequestPolicy{RequestTimeout: time.Second, StreamIdleTimeout: 70 * time.Millisecond}
			if fixedDeadline {
				policy.RequestTimeout = 85 * time.Millisecond
			}
			count := 0
			model := &Model{Policy: policy, LlmModel: functionLLM(func(ctx context.Context, _ []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
				call := resolveOptions(options)
				ticker := time.NewTicker(20 * time.Millisecond)
				defer ticker.Stop()
				for count < 6 {
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-ticker.C:
						count++
						if err := call.StreamingFunc(ctx, []byte("x")); err != nil {
							return nil, err
						}
					}
				}
				return responseWithText(strings.Repeat("x", count)), nil
			})}
			_, err := model.ChatWithStreamCtx(context.Background(), nil)
			if fixedDeadline {
				if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrStreamIdleTimeout) || count >= 6 {
					t.Fatalf("absolute deadline extended: err=%v chunks=%d", err, count)
				}
			} else if err != nil || count != 6 {
				t.Fatalf("active long stream failed: err=%v chunks=%d", err, count)
			}
		})
	}
}

func TestRetryClassification(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		transient bool
	}{
		{"rate limit", &RequestError{StatusCode: 429}, true},
		{"server error", &RequestError{StatusCode: 503}, true},
		{"other server error", &RequestError{StatusCode: 599}, true},
		{"authentication", &RequestError{StatusCode: 401}, false},
		{"bad input", &RequestError{StatusCode: 400}, false},
		{"network", &net.OpError{Op: "read", Net: "tcp", Err: io.ErrUnexpectedEOF}, true},
		{"network timeout", &net.DNSError{IsTimeout: true}, true},
		{"short response", io.ErrUnexpectedEOF, true},
		{"canceled", context.Canceled, false},
		{"deadline", context.DeadlineExceeded, false},
		{"certificate", x509.UnknownAuthorityError{}, false},
		{"google server error", &googleapi.Error{Code: 503}, true},
		{"ordinary error", errors.New("validation error"), false},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if got := IsTransient(fmt.Errorf("wrapped: %w", item.err)); got != item.transient {
				t.Fatalf("transient=%v want=%v", got, item.transient)
			}
		})
	}
}

func TestRetryBackoffHonorsHintsWithinBudget(t *testing.T) {
	now := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	policy := RequestPolicy{InitialBackoff: 250 * time.Millisecond, MaxBackoff: 4 * time.Second}
	cases := []struct {
		retry int
		hint  string
		want  time.Duration
		ok    bool
	}{
		{0, "", 250 * time.Millisecond, true},
		{1, "", 500 * time.Millisecond, true},
		{4, "", 4 * time.Second, true},
		{10, "", 4 * time.Second, true},
		{0, "2", 2 * time.Second, true},
		{0, now.Add(3 * time.Second).Format(http.TimeFormat), 3 * time.Second, true},
		{0, now.Add(-time.Second).Format(http.TimeFormat), 250 * time.Millisecond, true},
		{0, "invalid", 250 * time.Millisecond, true},
		{0, "9999999999999999999", 0, false},
		{0, "999999999999999999999999999999999", 0, false},
		{0, "5", 0, false},
	}
	for _, item := range cases {
		got, ok := retryDelay(policy, item.retry, &RequestError{StatusCode: 429, RetryAfter: item.hint}, now)
		if got != item.want || ok != item.ok {
			t.Errorf("retry=%d hint=%q: got=%v,%v want=%v,%v", item.retry, item.hint, got, ok, item.want, item.ok)
		}
	}
}

func TestModelRetriesOnlyBeforeStreamData(t *testing.T) {
	for _, mode := range []string{"before stream", "answer", "reasoning", "tool", "hidden tool bytes", "non transient"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			var answers strings.Builder
			model := &Model{Policy: RequestPolicy{InitialBackoff: time.Millisecond, MaxBackoff: 5 * time.Millisecond}, LlmModel: functionLLM(func(ctx context.Context, _ []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
				calls++
				if calls == 1 {
					call := resolveOptions(options)
					switch mode {
					case "answer":
						_ = call.StreamingFunc(ctx, []byte("partial"))
					case "reasoning":
						_ = call.StreamingReasoningFunc(ctx, []byte("thinking"), nil)
					case "tool":
						_ = call.StreamingFunc(ctx, []byte(`[{"id":"call","type":"function","function":{"name":"read_file","arguments":"{"}}]`))
					case "hidden tool bytes":
						ObserveHTTPResponse(ctx, 200, "")
						ObserveResponseRead(ctx, 4, nil)
					case "non transient":
						return nil, &RequestError{StatusCode: 400}
					}
					return nil, &RequestError{StatusCode: 503}
				}
				return responseWithText("ok"), nil
			})}
			result, err := model.ChatWithStreamToolsHandlerCtx(context.Background(), nil, []llms.Tool{{Type: "function"}}, ChatStreamHandler{OnAnswer: func(text string) { answers.WriteString(text) }})
			if mode == "before stream" {
				if err != nil || calls != 2 || result.Content != "ok" || answers.String() != "ok" {
					t.Fatalf("err=%v calls=%d result=%+v answers=%q", err, calls, result, answers.String())
				}
			} else if err == nil || calls != 1 || len(result.ToolCalls) != 0 {
				t.Fatalf("unsafe retry: err=%v calls=%d result=%+v", err, calls, result)
			}
		})
	}
}

func TestModelRetryLimitsAndCancellationDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	model := &Model{Policy: RequestPolicy{InitialBackoff: time.Second}, LlmModel: functionLLM(func(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
		calls++
		cancel()
		return nil, &RequestError{StatusCode: 503}
	})}
	if _, err := model.ChatCtx(ctx, nil); !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	calls = 0
	model.Policy = RequestPolicy{InitialBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond}
	model.LlmModel = functionLLM(func(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
		calls++
		return nil, &RequestError{StatusCode: 503}
	})
	if _, err := model.ChatCtx(context.Background(), nil); err == nil || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
