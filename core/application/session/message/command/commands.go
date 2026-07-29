package command

import domainmessage "myai/core/domain/message"

type AppendUserMessage struct {
	SessionID       string
	Input           string
	ForceChatMode   bool
	RAGContext      string
	SyntheticReason domainmessage.SyntheticReason
}

type PrepareRegeneration struct {
	SessionID string
}
