package command

import (
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	modelport "myai/core/port/model"
	"myai/core/session"
)

type PersistUserMessage struct {
	SessionID string
	Model     string
	Title     string
	Input     string
}

type ToolExecution struct {
	Session   *session.Session
	Calls     []domainmessage.ToolCall
	Stream    modelport.ChatStreamHandler
	RequestID string
}

type ToolExecutionRecord struct {
	Entries []domaintool.ExecutionEntry
	Assets  []domaintool.SharedAsset
}
