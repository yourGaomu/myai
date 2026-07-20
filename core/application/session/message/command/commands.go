package command

type AppendUserMessage struct {
	SessionID     string
	Input         string
	ForceChatMode bool
	RAGContext    string
}

type PrepareRegeneration struct {
	SessionID string
}
