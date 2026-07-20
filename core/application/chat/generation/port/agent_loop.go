package port

import (
	"context"

	generationcommand "myai/core/application/chat/generation/command"
	generationresult "myai/core/application/chat/generation/result"
	"myai/core/contextmgr"
	modelport "myai/core/port/model"
	"myai/core/session"
)

type ContextProvider interface {
	Snapshot(current *session.Session) contextmgr.Snapshot
}

type ToolCatalog interface {
	ToolsForSession(current *session.Session, forceChatMode bool) []modelport.Tool
}

type ToolExecutor interface {
	Execute(ctx context.Context, command generationcommand.ToolExecution) (generationresult.ToolExecution, error)
}

type ToolExecutionRecordSink interface {
	RecordToolExecution(ctx context.Context, command generationcommand.ToolExecutionRecord)
}
