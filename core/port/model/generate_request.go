package model

import (
	generation "myai/core/domain/generation"
	domainmessage "myai/core/domain/message"
)

type GenerateRequest struct {
	Messages []domainmessage.Message
	Tools    []Tool
	Stream   ChatStreamHandler
	Settings generation.ResolvedSettings
}
