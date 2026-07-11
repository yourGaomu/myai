package command

import (
	modelport "myai/core/port/model"
	"myai/core/session"
)

type Run struct {
	Model         modelport.ChatModelPort
	Session       *session.Session
	Stream        modelport.ChatStreamHandler
	RuntimePrompt string
	LatestInput   string
	RequestID     string
	ForceChatMode bool
}
