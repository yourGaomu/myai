package port

import (
	domaingeneration "myai/core/domain/generation"
	domainsubagent "myai/core/domain/subagent"
)

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

// StructuredPendingTurnInput preserves the full inter-agent envelope while it
// is waiting for the recipient turn. It is optional so ordinary local input
// queues remain small and easy to test.
type StructuredPendingTurnInput interface {
	// 1. 入参必须携带完整 AgentMessage，不能在端口层退化成匿名文本。
	EnqueueAgentMessage(sessionID string, message domainsubagent.AgentMessage) error
}

type PendingInputAcknowledger interface {
	Acknowledge(messageID string) error
}

// PendingInputReleaser puts claimed messages back at the head of a session
// queue when the model turn fails before the input was consumed.
type PendingInputReleaser interface {
	RequeueIdentified(sessionID string, items []domaingeneration.PendingTurnInputItem) error
}

// ImmediatePendingTurnInput lets AgentLoop distinguish Codex-style
// steer-current-turn input from queue-only mailbox messages. Legacy queues do
// not implement it and retain their historical immediate-delivery behavior.
type ImmediatePendingTurnInput interface {
	// 2. 只有 steer 消息可以在当前 turn 重新打开 mailbox 投递窗口。
	HasImmediatePending(sessionID string) bool
}

type PendingInputConfigurer interface {
	SetAcknowledger(acknowledger PendingInputAcknowledger)
}
