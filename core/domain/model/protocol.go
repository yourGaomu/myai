package model

import "strings"

type Protocol string

const (
	ProtocolOpenAIChatCompletions Protocol = "openai-chat-completions"
)

type AuthType string

const (
	AuthTypeBearer AuthType = "bearer"
	AuthTypeNone   AuthType = "none"
)

func NormalizeProtocol(protocol Protocol) Protocol {
	protocol = Protocol(strings.ToLower(strings.TrimSpace(string(protocol))))
	if protocol == "" {
		return ProtocolOpenAIChatCompletions
	}
	return protocol
}

func NormalizeAuthType(authType AuthType) AuthType {
	authType = AuthType(strings.ToLower(strings.TrimSpace(string(authType))))
	if authType == "" {
		return AuthTypeBearer
	}
	return authType
}
