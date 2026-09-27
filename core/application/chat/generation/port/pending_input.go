package port

import domaingeneration "myai/core/domain/generation"

type PendingTurnInput interface {
	Enqueue(sessionID, content string) error
	Drain(sessionID string) []string
	HasPending(sessionID string) bool
}

// IdentifiedPendingTurnInput is an optional extension used by durable
// inter-agent messages. Existing callers can continue using PendingTurnInput.
type IdentifiedPendingTurnInput interface {
	PendingTurnInput
	EnqueueIdentified(sessionID, messageID, content string) error
	DrainIdentified(sessionID string) []domaingeneration.PendingTurnInputItem
	Acknowledge(messageID string) error
}

type PendingInputAcknowledger interface {
	Acknowledge(messageID string) error
}

// PendingInputReleaser puts claimed messages back at the head of a session
// queue when the model turn fails before the input was consumed.
type PendingInputReleaser interface {
	RequeueIdentified(sessionID string, items []domaingeneration.PendingTurnInputItem) error
}

type PendingInputConfigurer interface {
	SetAcknowledger(acknowledger PendingInputAcknowledger)
}
