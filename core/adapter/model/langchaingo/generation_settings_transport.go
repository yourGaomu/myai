package langchaingo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	domainmodel "myai/core/domain/model"
	corellm "myai/core/llm"
)

// generationSettingsTransport restores settings dropped by this SDK version.
// Mistral uses the legacy max_tokens field. OpenAI cache routing is enabled
// only for its official endpoint; message and tool values remain unchanged.
type generationSettingsTransport struct {
	base             http.RoundTripper
	legacyMaxTokens  bool
	promptCacheModel string
}

func (t generationSettingsTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	topP, hasTopP := corellm.RequestTopP(request.Context())
	if request.Body == nil || !strings.HasSuffix(request.URL.Path, "/chat/completions") || !hasTopP && !t.legacyMaxTokens && t.promptCacheModel == "" {
		return t.base.RoundTrip(request)
	}
	body, err := io.ReadAll(request.Body)
	request.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read generation settings: %w", err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode generation settings: %w", err)
	}
	if hasTopP {
		payload["top_p"], _ = json.Marshal(topP)
	}
	if t.legacyMaxTokens {
		if tokens, exists := payload["max_completion_tokens"]; exists {
			payload["max_tokens"] = tokens
			delete(payload, "max_completion_tokens")
		}
	}
	if t.promptCacheModel != "" && isOfficialOpenAIPromptCacheEndpoint(request.URL) {
		if key, ok := openAIPromptCacheKey(payload, t.promptCacheModel); ok {
			payload["prompt_cache_key"], _ = json.Marshal(key)
		}
	}
	body, err = json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	clone := request.Clone(request.Context())
	clone.Body = io.NopCloser(bytes.NewReader(body))
	clone.ContentLength = int64(len(body))
	clone.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	return t.base.RoundTrip(clone)
}

// isOfficialOpenAIPromptCacheEndpoint verifies the actual request destination as well as the configured base URL.
func isOfficialOpenAIPromptCacheEndpoint(endpoint *url.URL) bool {
	return endpoint != nil && strings.EqualFold(endpoint.Scheme, "https") && strings.EqualFold(endpoint.Hostname(), "api.openai.com")
}

// officialOpenAIPromptCacheModel returns a model name only for the official
// OpenAI HTTPS endpoint. OpenAI-compatible relays may reject provider-specific
// fields, so they keep their existing request payload unchanged.
func officialOpenAIPromptCacheModel(baseURL string, protocol domainmodel.Protocol, model string) string {
	if protocol != domainmodel.ProtocolOpenAIChatCompletions {
		return ""
	}
	endpoint, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || !isOfficialOpenAIPromptCacheEndpoint(endpoint) {
		return ""
	}
	return strings.TrimSpace(model)
}

// openAIPromptCacheKey groups requests by the stable model, leading system
// instructions, and tool schemas. User/assistant history is deliberately
// excluded so sessions sharing a stable prompt prefix can use the same route.
func openAIPromptCacheKey(payload map[string]json.RawMessage, model string) (string, bool) {
	var messages []json.RawMessage
	if raw := payload["messages"]; len(raw) != 0 {
		if err := json.Unmarshal(raw, &messages); err != nil {
			return "", false
		}
	}
	stableMessages := make([]json.RawMessage, 0, len(messages))
	for _, message := range messages {
		var header struct {
			Role string `json:"role"`
		}
		if err := json.Unmarshal(message, &header); err != nil {
			return "", false
		}
		if header.Role != "system" && header.Role != "developer" {
			break
		}
		canonical, ok := canonicalJSON(message)
		if !ok {
			return "", false
		}
		stableMessages = append(stableMessages, canonical)
	}

	var tools json.RawMessage
	if raw := payload["tools"]; len(raw) != 0 {
		canonical, ok := canonicalJSON(raw)
		if !ok {
			return "", false
		}
		tools = canonical
	}
	if len(stableMessages) == 0 && len(tools) == 0 {
		return "", false
	}
	material, err := json.Marshal(struct {
		Model    string            `json:"model"`
		Messages []json.RawMessage `json:"messages,omitempty"`
		Tools    json.RawMessage   `json:"tools,omitempty"`
	}{Model: model, Messages: stableMessages, Tools: tools})
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(material)
	return hex.EncodeToString(sum[:]), true
}

func canonicalJSON(raw json.RawMessage) (json.RawMessage, bool) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, false
	}
	canonical, err := json.Marshal(value)
	return canonical, err == nil
}
