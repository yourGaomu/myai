package command

import (
	generation "myai/core/domain/generation"
	modelport "myai/core/port/model"
	"myai/core/session"
)

type Run struct {
	Model              modelport.ChatModelPort
	Session            *session.Session
	Stream             modelport.ChatStreamHandler
	RequestID          string
	ForceChatMode      bool
	Settings           generation.ResolvedSettings
	MemoryContext      string
	EnvironmentContext string
}
