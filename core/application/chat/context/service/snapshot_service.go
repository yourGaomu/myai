package service

import (
	chatcontextapi "myai/core/application/chat/context/api"
	runtimeservice "myai/core/application/runtime/service"
	"myai/core/contextmgr"
	domainmessage "myai/core/domain/message"
	"myai/core/session"
)

type SnapshotService struct{}

var _ chatcontextapi.SnapshotService = SnapshotService{}

func (SnapshotService) Snapshot(current *session.Session, runtimePrompt string) contextmgr.Snapshot {
	if current == nil {
		return contextmgr.Snapshot{}
	}
	// 先注入仅本轮有效的动态指令，再由 contextmgr 按摘要和窗口预算选择历史消息。
	return contextmgr.BuildSnapshot(SnapshotService{}.MessagesWithRuntimePrompt(current.Messages, runtimePrompt), current.Summary, current.CompactedMessages, current.ContextWindowK)
}

func (SnapshotService) MessagesWithRuntimePrompt(messages []domainmessage.Message, runtimePrompt string) []domainmessage.Message {
	return runtimeservice.InsertRuntimeInstructions(messages, runtimePrompt)
}
