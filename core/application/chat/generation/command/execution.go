package command

import (
	"time"

	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	modelport "myai/core/port/model"
	"myai/core/session"
)

type PersistUserMessage struct {
	SessionID          string
	Model              string
	Title              string
	Input              string
	RuntimeInstruction string
	RAGContext         string
	SessionSnapshot    *session.Session
	CreatedAt          time.Time
}

type ToolExecution struct {
	Session       *session.Session
	Calls         []domainmessage.ToolCall
	Stream        modelport.ChatStreamHandler
	RequestID     string
	ForceChatMode bool
}

type ToolExecutionRecord struct {
	Entries []domaintool.ExecutionEntry
	Assets  []domaintool.SharedAsset
}
