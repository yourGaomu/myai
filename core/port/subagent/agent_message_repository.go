package subagent

import (
	"context"
	"time"

	domainsubagent "myai/core/domain/subagent"
)

// AgentMessageRepository stores parent-child messages independently from the
// transient execution queue. A message remains pending until the recipient
// session has appended it to its durable conversation.
type AgentMessageRepository interface {
	SaveAgentMessage(ctx context.Context, message domainsubagent.AgentMessage) error
	GetAgentMessage(ctx context.Context, messageID string) (domainsubagent.AgentMessage, error)
	ListPendingAgentMessages(ctx context.Context, recipientAgentID string, limit int) ([]domainsubagent.AgentMessage, error)
	ListPendingParentMessages(ctx context.Context) ([]domainsubagent.AgentMessage, error)
	MarkAgentMessageDelivered(ctx context.Context, messageID string, deliveredAt time.Time) error
	MarkAgentMessageFailed(ctx context.Context, messageID string, reason string) error
}

type ParentCompletionMessageRepository interface {
	EnqueueParentCompletion(ctx context.Context, taskID string, message domainsubagent.AgentMessage) (domainsubagent.AgentMessage, error)
	ClaimParentCompletion(ctx context.Context, messageID, ownerID string, now time.Time, ttl time.Duration) (domainsubagent.AgentMessage, error)
	ReleaseParentCompletion(ctx context.Context, messageID, ownerID, reason string) error
	CompleteParentCompletion(ctx context.Context, messageID, ownerID string, deliveredAt time.Time) error
}

// ChildAgentMessageRepository keeps the durable message and the child task's
// mailbox projection consistent. Implementations must make each operation
// atomic, including retries with the same message ID.
type ChildAgentMessageRepository interface {
	EnqueueChildAgentMessage(ctx context.Context, taskID string, message domainsubagent.AgentMessage) (domainsubagent.Task, error)
	ClaimChildMailboxMessage(ctx context.Context, taskID string, now time.Time) (domainsubagent.Message, bool, error)
	AcknowledgeChildAgentMessage(ctx context.Context, taskID, messageID string, deliveredAt time.Time) error
	ReleaseChildMailboxMessage(ctx context.Context, taskID, messageID, reason string, now time.Time) error
}
