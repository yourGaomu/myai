package command

import (
	modelport "myai/core/port/model"
	"myai/core/session"
)

type GenerationTask struct {
	Session       *session.Session
	LatestInput   string
	Title         string
	Reason        string
	Stream        modelport.ChatStreamHandler
	CapturePlan   bool
	ForceChatMode bool
}

type TaskRecord struct {
	Title     string
	Reason    string
	SessionID string
	RequestID string
}
