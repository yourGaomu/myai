package command

import domainmessage "myai/core/domain/message"

type AppendUserMessage struct {
	SessionID               string
	Input                   string
	ForceChatMode           bool
	ForceAutonomousPlanning bool
	RAGContext              string
	SyntheticReason         domainmessage.SyntheticReason
	// 1. SourceID 是生成消息的稳定事件身份，用于幂等追加和冲突检测。
	SourceID     string
	SourceKind   string
	SourceTaskID string
}

type PrepareRegeneration struct {
	SessionID string
}
