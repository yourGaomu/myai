package langchaingo

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	corellm "myai/core/llm"
)

// modelTransport observes failures but never retries HTTP requests itself. Only
// the model boundary knows whether a stream callback has already been delivered.
type modelTransport struct {
	base   http.RoundTripper
	ollama bool
	usage  usageProtocol
}

func (t modelTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	response, err := base.RoundTrip(request)
	if err != nil {
		corellm.ObserveTransportError(request.Context(), err)
		return response, err
	}
	corellm.ObserveHTTPResponse(request.Context(), response.StatusCode, response.Header.Get("Retry-After"))
	if response.StatusCode >= 400 {
		// Return a typed error before the SDK flattens it or performs a hidden
		// status retry. The outer model owns the only retry budget.
		var cause error
		if response.Body != nil {
			if response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusRequestEntityTooLarge || response.StatusCode == http.StatusUnprocessableEntity {
				body, _ := io.ReadAll(io.LimitReader(response.Body, 16*1024))
				if contextWindowErrorCode(body) {
					cause = corellm.ErrContextWindowExceeded
				}
			}
			response.Body.Close()
		}
		failure := &corellm.RequestError{StatusCode: response.StatusCode, RetryAfter: response.Header.Get("Retry-After"), Err: cause}
		corellm.ObserveRequestFailure(request.Context(), failure)
		return nil, failure
	}
	if response.Body != nil {
		body := &observedResponseBody{ReadCloser: response.Body, ctx: request.Context(), ollama: t.ollama}
		if response.StatusCode == http.StatusOK && t.usage != noUsageObservation {
			ctx := request.Context()
			body.usage = &usageObserver{ctx: ctx, protocol: t.usage, streaming: corellm.RequestStreaming(ctx)}
			corellm.ObserveTokenUsage(ctx, corellm.TokenUsage{})
		}
		response.Body = body
	}
	return response, nil
}

// Only structured provider codes identify an overflow. Generic 400s and prose
// messages are not classified as context errors or made retryable.
func contextWindowErrorCode(body []byte) bool {
	var payload struct {
		Code  string `json:"code"`
		Type  string `json:"type"`
		Error struct {
			Code string `json:"code"`
			Type string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return false
	}
	for _, code := range []string{payload.Code, payload.Type, payload.Error.Code, payload.Error.Type} {
		switch strings.ToLower(code) {
		case "context_length_exceeded", "context_window_exceeded", "prompt_too_long", "input_too_long":
			return true
		}
	}
	return false
}

type observedResponseBody struct {
	io.ReadCloser
	ctx      context.Context
	ollama   bool
	usage    *usageObserver
	line     []byte
	dropping bool
}

func (b *observedResponseBody) Read(data []byte) (int, error) {
	n, err := b.ReadCloser.Read(data)
	corellm.ObserveResponseRead(b.ctx, n, err)
	if b.usage != nil {
		b.usage.read(data[:n], err == io.EOF)
	}
	if b.ollama {
		b.observeOllama(data[:n])
		if err == io.EOF {
			b.observeOllama([]byte{'\n'})
		}
	}
	return n, err
}

// Ollama's SDK drops done_reason and ignores Scanner.Err. Observe just the
// terminal NDJSON metadata, with a bounded buffer; payload bytes pass unchanged.
func (b *observedResponseBody) observeOllama(data []byte) {
	for len(data) > 0 {
		end := bytes.IndexByte(data, '\n')
		if end < 0 {
			end = len(data)
		}
		if !b.dropping {
			if len(b.line)+end > 512*1024 {
				b.line = b.line[:0]
				b.dropping = true
				corellm.ObserveResponseRead(b.ctx, 0, corellm.ErrIncompleteResponse)
			} else {
				b.line = append(b.line, data[:end]...)
			}
		}
		if end == len(data) {
			return
		}
		if !b.dropping {
			var frame struct {
				Done       bool            `json:"done"`
				DoneReason string          `json:"done_reason"`
				Message    json.RawMessage `json:"message"`
			}
			if json.Unmarshal(b.line, &frame) == nil {
				if len(frame.Message) > 0 {
					corellm.ObserveResponseChunk(b.ctx)
				}
				if frame.Done {
					reason := frame.DoneReason
					if reason == "" {
						reason = "stop"
					}
					corellm.ObserveStopReason(b.ctx, reason)
				}
			}
		}
		b.line = b.line[:0]
		b.dropping = false
		data = data[end+1:]
	}
}
