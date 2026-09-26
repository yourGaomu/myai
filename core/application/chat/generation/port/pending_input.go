package port

type PendingTurnInput interface {
	Enqueue(sessionID, content string) error
	Drain(sessionID string) []string
	HasPending(sessionID string) bool
}
