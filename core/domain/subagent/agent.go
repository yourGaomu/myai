package subagent

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// AgentThread is the durable identity of a child agent. Task remains as the
// storage-facing name while the runtime is migrated to the Thread/Turn model
// used by Codex.
type AgentThread = Task

// AgentTurn is one execution attempt for an AgentThread. Run remains as the
// storage-facing name during the incremental migration.
type AgentTurn = Run

// AgentStatus describes the lifecycle of an AgentThread independently from
// any single execution turn.
type AgentStatus string

const (
	AgentStatusCreated     AgentStatus = "created"
	AgentStatusQueued      AgentStatus = "queued"
	AgentStatusRunning     AgentStatus = "running"
	AgentStatusWaiting     AgentStatus = "waiting"
	AgentStatusIdle        AgentStatus = "idle"
	AgentStatusCompleted   AgentStatus = "completed"
	AgentStatusFailed      AgentStatus = "failed"
	AgentStatusInterrupted AgentStatus = "interrupted"
	AgentStatusUnloaded    AgentStatus = "unloaded"
)

// AgentRuntimeSnapshot describes process-local residency and admission state
// without changing the durable AgentThread record.
type AgentRuntimeSnapshot struct {
	ThreadID       string
	ParentThreadID string
	RootAgentID    string
	AgentPath      string
	AgentNickname  string
	CurrentTurnID  string
	Status         AgentStatus
	Loaded         bool
}

// AgentMessageKind identifies the semantic purpose of an inter-agent message.
type AgentMessageKind string

const (
	AgentMessageKindUserInput  AgentMessageKind = "user_input"
	AgentMessageKindAssignment AgentMessageKind = "task_assignment"
	AgentMessageKindInterAgent AgentMessageKind = "inter_agent_message"
	AgentMessageKindTaskResult AgentMessageKind = "task_result"
	AgentMessageKindTaskError  AgentMessageKind = "task_error"
	AgentMessageKindControl    AgentMessageKind = "control"
)

// AgentMessageTrigger controls when a recipient may consume a message.
type AgentMessageTrigger string

const (
	AgentMessageTriggerQueue AgentMessageTrigger = "queue"
	AgentMessageTriggerTurn  AgentMessageTrigger = "trigger_turn"
	AgentMessageTriggerSteer AgentMessageTrigger = "steer_current_turn"
)

type AgentMessageDeliveryStatus string

const (
	AgentMessagePending    AgentMessageDeliveryStatus = "pending"
	AgentMessageDelivering AgentMessageDeliveryStatus = "delivering"
	AgentMessageDelivered  AgentMessageDeliveryStatus = "delivered"
	AgentMessageFailed     AgentMessageDeliveryStatus = "failed"
)

// AgentMessage is the durable envelope used for parent-child communication.
// It deliberately carries identity and delivery metadata instead of treating
// every message as an anonymous string in a mailbox.
type AgentMessage struct {
	ID               string
	SourceTaskID     string
	AuthorAgentID    string
	RecipientAgentID string
	ParentTurnID     string
	RootAgentID      string
	Kind             AgentMessageKind
	Content          string
	Trigger          AgentMessageTrigger
	Sequence         uint64
	Status           AgentMessageDeliveryStatus
	DeliveryAttempts int
	LastError        string
	CreatedAt        time.Time
	DeliveredAt      *time.Time
	ClaimOwnerID     string
	ClaimExpiresAt   *time.Time
}

func AgentResultMessageID(taskID string) string {
	return "agent-result:" + strings.TrimSpace(taskID)
}

func (message AgentMessage) Validate() error {
	if strings.TrimSpace(message.ID) == "" {
		return errors.New("agent message id is required")
	}
	if strings.TrimSpace(message.RecipientAgentID) == "" {
		return errors.New("agent message recipient is required")
	}
	if strings.TrimSpace(message.Content) == "" {
		return errors.New("agent message content is required")
	}
	if message.Kind == "" {
		return errors.New("agent message kind is required")
	}
	if message.Trigger == "" {
		return errors.New("agent message trigger is required")
	}
	if message.Status == "" {
		return errors.New("agent message status is required")
	}
	switch message.Kind {
	case AgentMessageKindUserInput, AgentMessageKindAssignment,
		AgentMessageKindInterAgent, AgentMessageKindTaskResult,
		AgentMessageKindTaskError, AgentMessageKindControl:
	default:
		return fmt.Errorf("unsupported agent message kind %q", message.Kind)
	}
	switch message.Trigger {
	case AgentMessageTriggerQueue, AgentMessageTriggerTurn, AgentMessageTriggerSteer:
	default:
		return fmt.Errorf("unsupported agent message trigger %q", message.Trigger)
	}
	switch message.Status {
	case AgentMessagePending, AgentMessageDelivering, AgentMessageDelivered, AgentMessageFailed:
	default:
		return fmt.Errorf("unsupported agent message status %q", message.Status)
	}
	return nil
}

func CloneAgentMessage(message AgentMessage) AgentMessage {
	if message.DeliveredAt != nil {
		value := *message.DeliveredAt
		message.DeliveredAt = &value
	}
	if message.ClaimExpiresAt != nil {
		value := *message.ClaimExpiresAt
		message.ClaimExpiresAt = &value
	}
	return message
}

func (message AgentMessage) SameRequest(other AgentMessage) bool {
	leftTrigger := message.Trigger
	if leftTrigger == "" {
		leftTrigger = AgentMessageTriggerQueue
	}
	rightTrigger := other.Trigger
	if rightTrigger == "" {
		rightTrigger = AgentMessageTriggerQueue
	}
	return message.ID == other.ID && message.SourceTaskID == other.SourceTaskID &&
		message.AuthorAgentID == other.AuthorAgentID && message.RecipientAgentID == other.RecipientAgentID &&
		message.ParentTurnID == other.ParentTurnID && message.RootAgentID == other.RootAgentID &&
		message.Kind == other.Kind && message.Content == other.Content && leftTrigger == rightTrigger
}
