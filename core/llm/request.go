package llm

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tmc/langchaingo/llms"
	"google.golang.org/api/googleapi"

	modelport "myai/core/port/model"
)

const (
	DefaultRequestTimeout    = 10 * time.Minute
	DefaultStreamIdleTimeout = 90 * time.Second
)

// RequestPolicy bounds one generation, including retries. Zero values select
// defaults; MaxAttempts=1 disables retries. The absolute deadline is never reset
// by stream activity.
type RequestPolicy struct {
	RequestTimeout    time.Duration
	StreamIdleTimeout time.Duration
	MaxAttempts       int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
}

func (p RequestPolicy) resolved() RequestPolicy {
	if p.RequestTimeout <= 0 {
		p.RequestTimeout = DefaultRequestTimeout
	}
	if p.StreamIdleTimeout <= 0 {
		p.StreamIdleTimeout = DefaultStreamIdleTimeout
	}
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 3
	}
	if p.InitialBackoff <= 0 {
		p.InitialBackoff = 250 * time.Millisecond
	}
	if p.MaxBackoff <= 0 {
		p.MaxBackoff = 30 * time.Second
	}
	if p.InitialBackoff > p.MaxBackoff {
		p.InitialBackoff = p.MaxBackoff
	}
	return p
}

var (
	ErrStreamIdleTimeout     = fmt.Errorf("model stream idle timeout: %w", context.DeadlineExceeded)
	ErrContextWindowExceeded = modelport.ErrContextWindowExceeded
)

func IsContextWindowExceeded(err error) bool {
	return errors.Is(err, ErrContextWindowExceeded)
}

// RequestError retains machine-readable HTTP/network details without including
// provider response bodies or request URLs in the displayed error.
type RequestError struct {
	StatusCode int
	RetryAfter string
	Err        error
}

func (e *RequestError) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("model request returned HTTP %d", e.StatusCode)
	}
	return "model network request failed"
}

func (e *RequestError) Unwrap() error { return e.Err }

// IsTransient classifies failures eligible for retry before any stream chunk.
func IsTransient(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	status, _ := retryMetadata(err)
	if status != 0 {
		return status == http.StatusTooManyRequests || status >= 500 && status <= 599
	}
	var unknownAuthority x509.UnknownAuthorityError
	var invalidCertificate x509.CertificateInvalidError
	var hostnameError x509.HostnameError
	if errors.As(err, &unknownAuthority) || errors.As(err, &invalidCertificate) || errors.As(err, &hostnameError) {
		return false
	}
	var networkError net.Error
	return errors.As(err, &networkError) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)
}

func retryMetadata(err error) (int, string) {
	var requestError *RequestError
	if errors.As(err, &requestError) {
		return requestError.StatusCode, requestError.RetryAfter
	}
	var googleError *googleapi.Error
	if errors.As(err, &googleError) {
		return googleError.Code, googleError.Header.Get("Retry-After")
	}
	return 0, ""
}

func retryDelay(policy RequestPolicy, retry int, err error, now time.Time) (time.Duration, bool) {
	delay := policy.InitialBackoff
	for i := 0; i < retry && delay < policy.MaxBackoff; i++ {
		if delay > policy.MaxBackoff/2 {
			delay = policy.MaxBackoff
		} else {
			delay *= 2
		}
	}
	_, value := retryMetadata(err)
	if after, ok := parseRetryAfter(value, now); ok && after > delay {
		// A hint beyond our wait budget is not shortened into an early retry.
		if after > policy.MaxBackoff {
			return 0, false
		}
		delay = after
	}
	return delay, true
}

func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil {
		if seconds > uint64((1<<63-1)/int64(time.Second)) {
			return time.Duration(1<<63 - 1), true
		}
		return time.Duration(seconds) * time.Second, true
	} else if errors.Is(err, strconv.ErrRange) {
		return time.Duration(1<<63 - 1), true
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(time.Duration(0), date.Sub(now)), true
	}
	return 0, false
}

type requestTopPKey struct{}

// RequestTopP exposes the selected option to SDK compatibility transports.
func RequestTopP(ctx context.Context) (float64, bool) {
	topP, ok := ctx.Value(requestTopPKey{}).(float64)
	return topP, ok
}

type requestAttemptKey struct{}

// RequestStreaming reports whether the SDK call expects a streaming response.
func RequestStreaming(ctx context.Context) bool {
	attempt, ok := ctx.Value(requestAttemptKey{}).(*requestAttempt)
	return ok && attempt.idle > 0
}

type requestAttempt struct {
	mu         sync.Mutex
	cancel     context.CancelCauseFunc
	timer      *time.Timer
	idle       time.Duration
	lastActive time.Time
	closed     bool
	chunk      bool
	stopReason string
	statusCode int
	retryAfter string
	failure    error
	networkErr error
	bodyErr    error
	usage      *TokenUsage
}

func newRequestAttempt(ctx context.Context, idle time.Duration) (context.Context, *requestAttempt) {
	ctx, cancel := context.WithCancelCause(ctx)
	attempt := &requestAttempt{cancel: cancel, idle: idle, lastActive: time.Now()}
	if idle > 0 {
		attempt.timer = time.AfterFunc(idle, func() {
			attempt.mu.Lock()
			defer attempt.mu.Unlock()
			if attempt.closed {
				return
			}
			if remaining := attempt.idle - time.Since(attempt.lastActive); remaining > 0 {
				attempt.timer.Reset(remaining)
				return
			}
			attempt.cancel(ErrStreamIdleTimeout)
		})
	}
	return context.WithValue(ctx, requestAttemptKey{}, attempt), attempt
}

func (a *requestAttempt) close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	if a.timer != nil {
		a.timer.Stop()
	}
	a.cancel(context.Canceled)
}

func (a *requestAttempt) activity(chunk bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return
	}
	a.chunk = a.chunk || chunk
	a.lastActive = time.Now()
	if a.timer != nil {
		a.timer.Reset(a.idle)
	}
}

// ObserveHTTPResponse lets protocol transports retain status and Retry-After
// even when an SDK flattens its errors into text.
func ObserveHTTPResponse(ctx context.Context, statusCode int, retryAfter string) {
	if attempt, ok := ctx.Value(requestAttemptKey{}).(*requestAttempt); ok {
		attempt.mu.Lock()
		defer attempt.mu.Unlock()
		attempt.statusCode, attempt.retryAfter = statusCode, retryAfter
	}
}

// ObserveRequestFailure retains a structured provider failure category even if
// an SDK replaces the transport error with a generic string.
func ObserveRequestFailure(ctx context.Context, err error) {
	if attempt, ok := ctx.Value(requestAttemptKey{}).(*requestAttempt); ok {
		attempt.mu.Lock()
		defer attempt.mu.Unlock()
		attempt.failure = err
	}
}

// ObserveTransportError preserves the original network error for classification.
func ObserveTransportError(ctx context.Context, err error) {
	if attempt, ok := ctx.Value(requestAttemptKey{}).(*requestAttempt); ok {
		attempt.mu.Lock()
		defer attempt.mu.Unlock()
		attempt.networkErr = err
	}
}

// ObserveResponseRead counts actual HTTP progress, including reasoning, tool
// arguments and keepalives that an SDK might not send to its callbacks. A
// successful stream that has received any body bytes is never replayed.
func ObserveResponseRead(ctx context.Context, n int, err error) {
	if attempt, ok := ctx.Value(requestAttemptKey{}).(*requestAttempt); ok {
		if n > 0 {
			attempt.mu.Lock()
			// Conservatively prevent replay once any successful stream bytes
			// arrive: some SDKs swallow tool/keepalive deltas instead of
			// forwarding them to callbacks.
			streamData := attempt.idle > 0 && attempt.statusCode >= 200 && attempt.statusCode < 300
			attempt.mu.Unlock()
			attempt.activity(streamData)
		}
		if err != nil && !errors.Is(err, io.EOF) {
			attempt.mu.Lock()
			defer attempt.mu.Unlock()
			attempt.bodyErr = err
		}
	}
}

// ObserveStopReason retains terminal metadata discarded by some SDKs.
func ObserveStopReason(ctx context.Context, reason string) {
	if attempt, ok := ctx.Value(requestAttemptKey{}).(*requestAttempt); ok {
		attempt.mu.Lock()
		defer attempt.mu.Unlock()
		attempt.stopReason = reason
	}
}

// ObserveResponseChunk also covers tool deltas that an SDK does not forward.
func ObserveResponseChunk(ctx context.Context) {
	if attempt, ok := ctx.Value(requestAttemptKey{}).(*requestAttempt); ok {
		attempt.activity(true)
	}
}

// ObserveTokenUsage records a provider's wire-level accounting for this attempt.
// An empty observation is authoritative when the SDK synthesizes zero values.
func ObserveTokenUsage(ctx context.Context, usage TokenUsage) {
	if attempt, ok := ctx.Value(requestAttemptKey{}).(*requestAttempt); ok {
		attempt.mu.Lock()
		defer attempt.mu.Unlock()
		attempt.usage = &usage
	}
}

func (m *Model) generateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, *TokenUsage, error) {
	if m == nil || m.LlmModel == nil {
		return nil, nil, errors.New("model is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	policy := m.Policy.resolved()
	ctx, cancel := context.WithTimeout(ctx, policy.RequestTimeout)
	defer cancel()
	var call llms.CallOptions
	for _, option := range options {
		option(&call)
	}
	ctx = context.WithValue(ctx, requestTopPKey{}, call.TopP)
	streaming := call.StreamingFunc != nil || call.StreamingReasoningFunc != nil
	for attemptIndex := 0; ; attemptIndex++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		idle := time.Duration(0)
		if streaming {
			idle = policy.StreamIdleTimeout
		}
		attemptCtx, attempt := newRequestAttempt(ctx, idle)
		attemptOptions := append([]llms.CallOption(nil), options...)
		if call.StreamingFunc != nil {
			attemptOptions = append(attemptOptions, llms.WithStreamingFunc(func(_ context.Context, chunk []byte) error {
				if err := context.Cause(attemptCtx); err != nil {
					return err
				}
				attempt.activity(len(chunk) > 0)
				return call.StreamingFunc(attemptCtx, chunk)
			}))
		}
		if call.StreamingReasoningFunc != nil {
			attemptOptions = append(attemptOptions, llms.WithStreamingReasoningFunc(func(_ context.Context, reasoning, chunk []byte) error {
				if err := context.Cause(attemptCtx); err != nil {
					return err
				}
				attempt.activity(len(reasoning) > 0 || len(chunk) > 0)
				return call.StreamingReasoningFunc(attemptCtx, reasoning, chunk)
			}))
		}
		response, err := m.LlmModel.GenerateContent(attemptCtx, messages, attemptOptions...)
		attempt.mu.Lock()
		cause := context.Cause(attemptCtx)
		chunk := attempt.chunk
		stopReason := attempt.stopReason
		usage := attempt.usage
		if cause != nil {
			err = cause
		} else if attempt.statusCode >= 400 {
			cause := attempt.failure
			if cause == nil {
				cause = err
			}
			err = &RequestError{StatusCode: attempt.statusCode, RetryAfter: attempt.retryAfter, Err: cause}
		} else if attempt.networkErr != nil {
			err = &RequestError{Err: attempt.networkErr}
		} else if attempt.bodyErr != nil {
			err = &RequestError{Err: attempt.bodyErr}
		}
		attempt.mu.Unlock()
		attempt.close()
		if err == nil {
			if stopReason != "" && response != nil {
				for _, choice := range response.Choices {
					if choice != nil && stopReasonFromChoice(choice) == "" {
						choice.StopReason = stopReason
					}
				}
			}
			return response, usage, nil
		}
		if chunk || !IsTransient(err) || attemptIndex+1 >= policy.MaxAttempts {
			return nil, nil, err
		}
		delay, ok := retryDelay(policy, attemptIndex, err, time.Now())
		if !ok {
			return nil, nil, err
		}
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= delay {
			return nil, nil, err
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, nil, ctx.Err()
		case <-timer.C:
		}
	}
}
