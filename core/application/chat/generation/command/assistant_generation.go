package command

import (
	modelport "myai/core/port/model"
	"myai/core/session"
)

type AssistantGeneration struct {
	Session       *session.Session
	LatestInput   string
	RequestID     string
	Stream        modelport.ChatStreamHandler
	CapturePlan   bool
	ForceChatMode bool
}
