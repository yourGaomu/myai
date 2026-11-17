package langchaingo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	corellm "myai/core/llm"
)

// generationSettingsTransport restores settings dropped by this SDK version.
// Mistral uses the legacy max_tokens field; other providers keep their existing
// token field. The original message/tool serialization is preserved.
type generationSettingsTransport struct {
	base            http.RoundTripper
	legacyMaxTokens bool
}

func (t generationSettingsTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	topP, hasTopP := corellm.RequestTopP(request.Context())
	if request.Body == nil || !strings.HasSuffix(request.URL.Path, "/chat/completions") || !hasTopP && !t.legacyMaxTokens {
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
