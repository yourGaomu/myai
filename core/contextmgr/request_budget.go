package contextmgr

import (
	"fmt"

	domainmessage "myai/core/domain/message"
)

const DefaultSafetyTokens = 64

// RequestBudget is the shared accounting for a provider request. System and
// request-only context are part of Messages, not an additional reservation.
type RequestBudget struct {
	WindowTokens int
	ToolTokens   int
	OutputTokens int
	SafetyTokens int
}

func NewRequestBudget(windowTokens, toolTokens, outputTokens int) RequestBudget {
	if windowTokens <= 0 {
		windowTokens = DefaultWindowK * 1000
	}
	return RequestBudget{WindowTokens: windowTokens, ToolTokens: max(0, toolTokens), OutputTokens: max(0, outputTokens), SafetyTokens: DefaultSafetyTokens}
}

// ResolveWindowTokens treats zero as automatic. A session override can reduce
// model capacity; it can never make the model's declared window larger.
func ResolveWindowTokens(sessionWindowK, modelWindowTokens int) int {
	window := modelWindowTokens
	if window <= 0 {
		window = DefaultWindowK * 1000
	}
	if sessionWindowK > 0 {
		window = min(window, sessionWindowK*1000)
		if modelWindowTokens <= 0 {
			window = sessionWindowK * 1000
		}
	}
	return window
}

func (b RequestBudget) ReservedTokens() int {
	return max(0, b.ToolTokens) + max(0, b.OutputTokens) + max(0, b.SafetyTokens)
}

func (b RequestBudget) MessageLimit() int {
	return max(0, b.WindowTokens-b.ReservedTokens())
}

func (b RequestBudget) AvailableTokens(messages []domainmessage.Message) int {
	return max(0, b.MessageLimit()-EstimateMessagesTokens(messages))
}

func (b RequestBudget) Validate(messages []domainmessage.Message) error {
	tokens := EstimateMessagesTokens(messages)
	if b.WindowTokens <= 0 || tokens+b.ReservedTokens() > b.WindowTokens {
		return fmt.Errorf("context budget exceeded: messages=%d, tools=%d, output=%d, safety=%d, window=%d", tokens, b.ToolTokens, b.OutputTokens, b.SafetyTokens, b.WindowTokens)
	}
	return ValidateToolPairs(messages)
}

func (b RequestBudget) ValidateSnapshot(snapshot Snapshot) error {
	if snapshot.Info.DroppedMessages > 0 {
		return fmt.Errorf("context requires compaction: %d history messages would be omitted without a checkpoint", snapshot.Info.DroppedMessages)
	}
	return b.Validate(snapshot.Messages)
}

type TurnContext struct {
	RuntimeInstruction string
	RAGContext         string
	EnvironmentContext string
	PlanContext        string
}

type RequestOptions struct {
	Budget      RequestBudget
	TurnContext TurnContext
}
