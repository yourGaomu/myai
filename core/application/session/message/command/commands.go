package command

import domainmessage "myai/core/domain/message"

type AppendUserMessage struct {
	SessionID            string
	Input                string
	ForceChatMode        bool
	RAGContext           string
	SyntheticReason      domainmessage.SyntheticReason
	DeduplicateSynthetic bool
}

type PrepareRegeneration struct {
	SessionID string
}
