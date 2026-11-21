package langchaingo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
)

type anthropicCacheKey struct{}

type anthropicCachePrefix struct {
	system       string
	historyLimit int // exclusive wire message index
}

func withAnthropicCachePrefix(ctx context.Context, request modelport.GenerateRequest) context.Context {
	var prefix anthropicCachePrefix
	var system strings.Builder
	var wireIndex, firstVolatile int
	firstVolatile = -1
	leadingSystem, historySafe := true, true
	for _, message := range request.Messages {
		if message.Role == domainmessage.RoleSystem {
			// The SDK concatenates the first text part of each system message.
			// Retain the exact fixed prefix and leave the trailing system text unmarked.
			if leadingSystem && !message.IsSynthetic() && len(message.Parts) > 0 && message.Parts[0].Type == domainmessage.PartText {
				system.WriteString(message.Parts[0].Text)
			} else {
				leadingSystem, historySafe = false, false
			}
			continue
		}
		leadingSystem = false
		if message.IsSynthetic() && message.SyntheticReason != domainmessage.SyntheticReasonCompactionSummary && firstVolatile < 0 {
			firstVolatile = wireIndex
		}
		if historySafe && message.Role == domainmessage.RoleUser && !message.IsSynthetic() {
			// Only cache completed history before a genuine input. Never advance
			// into the active turn or past injected per-request context.
			prefix.historyLimit = wireIndex
			if firstVolatile >= 0 {
				prefix.historyLimit = firstVolatile
			}
		}
		wireIndex++
	}
	prefix.system = system.String()
	return context.WithValue(ctx, anthropicCacheKey{}, prefix)
}

type anthropicCacheTransport struct {
	base http.RoundTripper
}

var ephemeralCacheControl = json.RawMessage(`{"type":"ephemeral"}`)

func (t anthropicCacheTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Body == nil || request.Method != http.MethodPost || !strings.HasSuffix(request.URL.Path, "/messages") {
		return t.base.RoundTrip(request)
	}
	body, err := io.ReadAll(request.Body)
	request.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read Anthropic request: %w", err)
	}
	prefix, _ := request.Context().Value(anthropicCacheKey{}).(anthropicCachePrefix)
	body, err = addAnthropicCacheBreakpoints(body, prefix)
	if err != nil {
		return nil, fmt.Errorf("add Anthropic cache breakpoints: %w", err)
	}
	clone := request.Clone(request.Context())
	clone.Body = io.NopCloser(bytes.NewReader(body))
	clone.ContentLength = int64(len(body))
	clone.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	return t.base.RoundTrip(clone)
}

type anthropicBlock map[string]json.RawMessage

func (b anthropicBlock) stringField(key string) string {
	var value string
	_ = json.Unmarshal(b[key], &value)
	return value
}

func addAnthropicCacheBreakpoints(body []byte, prefix anthropicCachePrefix) ([]byte, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	var system string
	var tools []anthropicBlock
	var messages []struct {
		Role    string           `json:"role"`
		Content []anthropicBlock `json:"content"`
	}
	if raw := payload["system"]; len(raw) != 0 {
		if err := json.Unmarshal(raw, &system); err != nil {
			return nil, err
		}
	}
	if raw := payload["tools"]; len(raw) != 0 {
		if err := json.Unmarshal(raw, &tools); err != nil {
			return nil, err
		}
	}
	if raw := payload["messages"]; len(raw) != 0 {
		if err := json.Unmarshal(raw, &messages); err != nil {
			return nil, err
		}
	}
	// Count caller-supplied controls against Anthropic's four breakpoint limit.
	remaining := 4
	for _, tool := range tools {
		if tool["cache_control"] != nil {
			remaining--
		}
	}
	for _, message := range messages {
		for _, block := range message.Content {
			if block["cache_control"] != nil {
				remaining--
			}
		}
	}
	changed := false
	if remaining > 0 {
		if rest, ok := strings.CutPrefix(system, prefix.system); ok && strings.TrimSpace(prefix.system) != "" {
			type systemBlock struct {
				Type  string          `json:"type"`
				Text  string          `json:"text"`
				Cache json.RawMessage `json:"cache_control,omitempty"`
			}
			blocks := []systemBlock{{Type: "text", Text: prefix.system, Cache: ephemeralCacheControl}}
			if rest != "" {
				blocks = append(blocks, systemBlock{Type: "text", Text: rest})
			}
			payload["system"], _ = json.Marshal(blocks)
			changed = true
			remaining--
		} else if len(tools) > 0 && tools[len(tools)-1]["cache_control"] == nil {
			// Tools precede system and messages in Anthropic's cache key.
			tools[len(tools)-1]["cache_control"] = ephemeralCacheControl
			payload["tools"], _ = json.Marshal(tools)
			changed = true
			remaining--
		}
	}

	var first, latest anthropicBlock
	pending := make(map[string]bool)
scanHistory:
	for _, message := range messages[:min(prefix.historyLimit, len(messages))] {
		if len(message.Content) == 0 {
			break
		}
		for _, block := range message.Content {
			switch block.stringField("type") {
			case "tool_use":
				id := block.stringField("id")
				if id == "" || pending[id] {
					break scanHistory
				}
				pending[id] = true
			case "tool_result":
				id := block.stringField("tool_use_id")
				if !pending[id] {
					break scanHistory
				}
				delete(pending, id)
			default:
				if len(pending) != 0 {
					break scanHistory
				}
			}
		}
		boundary := message.Content[len(message.Content)-1]
		kind := boundary.stringField("type")
		if len(pending) != 0 || !(message.Role == "assistant" && kind == "text" && strings.TrimSpace(boundary.stringField("text")) != "" || kind == "tool_result") {
			continue
		}
		if first == nil {
			first = boundary
		}
		latest = boundary
	}
	// Keep the first history anchor as the rolling boundary advances. Anthropic
	// checks only a limited number of blocks backwards from each breakpoint.
	historyChanged := false
	for _, boundary := range []anthropicBlock{first, latest} {
		if remaining > 0 && boundary != nil && boundary["cache_control"] == nil {
			boundary["cache_control"] = ephemeralCacheControl
			historyChanged = true
			remaining--
		}
	}
	if historyChanged {
		payload["messages"], _ = json.Marshal(messages)
		changed = true
	}
	if !changed {
		return body, nil
	}
	return json.Marshal(payload)
}
