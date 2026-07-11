package model

import domainmessage "myai/core/domain/message"

type GenerateRequest struct {
	Messages []domainmessage.Message
	Tools    []Tool
	Stream   ChatStreamHandler
}
