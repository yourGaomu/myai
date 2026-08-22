package model

import "testing"

func TestNormalizeProtocolSupportsProviderAliases(t *testing.T) {
	cases := map[Protocol]Protocol{
		"":                  ProtocolOpenAIChatCompletions,
		"openai":            ProtocolOpenAIChatCompletions,
		"openai-compatible": ProtocolOpenAIChatCompletions,
		"anthropic":         ProtocolAnthropicMessages,
		"claude":            ProtocolAnthropicMessages,
		"gemini":            ProtocolGoogleGenerativeAI,
		"google":            ProtocolGoogleGenerativeAI,
		"mistral":           ProtocolMistralChat,
		"ollama":            ProtocolOllamaChat,
		" custom-protocol ": "custom-protocol",
	}
	for input, expected := range cases {
		if got := NormalizeProtocol(input); got != expected {
			t.Fatalf("NormalizeProtocol(%q) = %q, want %q", input, got, expected)
		}
	}
}
