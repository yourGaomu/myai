package model

import "strings"

type Protocol string

const (
	ProtocolOpenAIChatCompletions Protocol = "openai-chat-completions"
	ProtocolAnthropicMessages     Protocol = "anthropic-messages"
	ProtocolGoogleGenerativeAI    Protocol = "google-generative-ai"
	ProtocolMistralChat           Protocol = "mistral-chat"
	ProtocolOllamaChat            Protocol = "ollama-chat"
)

type AuthType string

const (
	AuthTypeBearer AuthType = "bearer"
	AuthTypeNone   AuthType = "none"
)

func NormalizeProtocol(protocol Protocol) Protocol {
	protocol = Protocol(strings.ToLower(strings.TrimSpace(string(protocol))))
	switch protocol {
	case "", "openai", "openai-compatible", "openai-chat", "chat-completions":
		return ProtocolOpenAIChatCompletions
	case "anthropic", "claude", "claude-messages":
		return ProtocolAnthropicMessages
	case "gemini", "google", "google-ai", "google-generative-ai":
		return ProtocolGoogleGenerativeAI
	case "mistral":
		return ProtocolMistralChat
	case "ollama":
		return ProtocolOllamaChat
	default:
		return protocol
	}
}

func NormalizeAuthType(authType AuthType) AuthType {
	authType = AuthType(strings.ToLower(strings.TrimSpace(string(authType))))
	if authType == "" {
		return AuthTypeBearer
	}
	return authType
}
