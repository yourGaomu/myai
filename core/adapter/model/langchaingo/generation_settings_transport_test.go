package langchaingo

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	domainmodel "myai/core/domain/model"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestOfficialOpenAIPromptCacheKeyIsEndpointGated(t *testing.T) {
	for _, test := range []struct {
		name     string
		baseURL  string
		protocol domainmodel.Protocol
		want     bool
	}{
		{name: "official endpoint", baseURL: "https://api.openai.com/v1", protocol: domainmodel.ProtocolOpenAIChatCompletions, want: true},
		{name: "official endpoint with port", baseURL: "https://api.openai.com:443/v1", protocol: domainmodel.ProtocolOpenAIChatCompletions, want: true},
		{name: "compatible relay", baseURL: "https://openai-relay.example/v1", protocol: domainmodel.ProtocolOpenAIChatCompletions},
		{name: "host suffix spoof", baseURL: "https://api.openai.com.example/v1", protocol: domainmodel.ProtocolOpenAIChatCompletions},
		{name: "insecure official host", baseURL: "http://api.openai.com/v1", protocol: domainmodel.ProtocolOpenAIChatCompletions},
		{name: "Mistral protocol", baseURL: "https://api.openai.com/v1", protocol: domainmodel.ProtocolMistralChat},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := officialOpenAIPromptCacheModel(test.baseURL, test.protocol, "gpt-5.5")
			if (got != "") != test.want {
				t.Fatalf("model=%q, want enabled=%v", got, test.want)
			}
		})
	}
}

func TestOpenAIPromptCacheKeyTracksStablePrefixOnly(t *testing.T) {
	payload := func(messages, tools string) map[string]json.RawMessage {
		return map[string]json.RawMessage{
			"messages": json.RawMessage(messages),
			"tools":    json.RawMessage(tools),
		}
	}
	base := payload(`[{"role":"system","content":"fixed policy"},{"role":"user","content":"first"}]`, `[{"type":"function","function":{"name":"read_file","parameters":{"type":"object"}}}]`)
	continued := payload(`[{"role":"system","content":"fixed policy"},{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":"next"}]`, `[{"type":"function","function":{"name":"read_file","parameters":{"type":"object"}}}]`)
	first, ok := openAIPromptCacheKey(base, "gpt-5.5")
	if !ok || len(first) != 64 {
		t.Fatalf("cache key=%q ok=%v", first, ok)
	}
	continuedKey, ok := openAIPromptCacheKey(continued, "gpt-5.5")
	if !ok || continuedKey != first {
		t.Fatalf("continued conversation changed stable-prefix key: %q != %q", continuedKey, first)
	}
	changedTools, ok := openAIPromptCacheKey(payload(`[{"role":"system","content":"fixed policy"},{"role":"user","content":"first"}]`, `[{"type":"function","function":{"name":"write_file","parameters":{"type":"object"}}}]`), "gpt-5.5")
	if !ok || changedTools == first {
		t.Fatal("tool schema change did not create a separate cache key")
	}
	changedSystem, ok := openAIPromptCacheKey(payload(`[{"role":"system","content":"different policy"},{"role":"user","content":"first"}]`, `[{"type":"function","function":{"name":"read_file","parameters":{"type":"object"}}}]`), "gpt-5.5")
	if !ok || changedSystem == first {
		t.Fatal("system prefix change did not create a separate cache key")
	}
}

func TestGenerationSettingsTransportInjectsStableOpenAIPromptCacheKey(t *testing.T) {
	var sent []byte
	transport := generationSettingsTransport{
		base: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			var err error
			sent, err = io.ReadAll(request.Body)
			if err != nil {
				return nil, err
			}
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`)), Request: request}, nil
		}),
		promptCacheModel: "gpt-5.5",
	}
	body := `{"model":"gpt-5.5","messages":[{"role":"system","content":"fixed policy"},{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"read_file","parameters":{"type":"object"}}}]}`
	request, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.RoundTrip(request); err != nil {
		t.Fatal(err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(sent, &payload); err != nil {
		t.Fatal(err)
	}
	var key string
	if err := json.Unmarshal(payload["prompt_cache_key"], &key); err != nil || len(key) != 64 {
		t.Fatalf("prompt_cache_key=%q err=%v body=%s", key, err, sent)
	}

	request, err = http.NewRequest(http.MethodPost, "https://relay.example/v1/chat/completions", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.RoundTrip(request); err != nil {
		t.Fatal(err)
	}
	payload = nil
	if err := json.Unmarshal(sent, &payload); err != nil {
		t.Fatal(err)
	}
	if _, exists := payload["prompt_cache_key"]; exists {
		t.Fatalf("compatible relay received OpenAI-specific cache key: %s", sent)
	}
}
