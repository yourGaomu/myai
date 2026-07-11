package command

type AppendUserMessage struct {
	SessionID string
	Input     string
}

type PrepareRegeneration struct {
	SessionID string
}
