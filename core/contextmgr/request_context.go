package contextmgr

import (
	"fmt"
	"strings"

	domainmessage "myai/core/domain/message"
)

// IsDynamicContext identifies request state. These messages may exist in old
// durable transcripts; projection must not delete them or change checkpoint
// source indexes and hashes.
func IsDynamicContext(message domainmessage.Message) bool {
	switch message.SyntheticReason {
	case domainmessage.SyntheticReasonRuntimeInstruction, domainmessage.SyntheticReasonProjectInstruction,
		domainmessage.SyntheticReasonSkillInstruction, domainmessage.SyntheticReasonRAGContext,
		domainmessage.SyntheticReasonMemoryContext, domainmessage.SyntheticReasonEnvironmentContext,
		domainmessage.SyntheticReasonPlanContext:
		return true
	}
	return false
}

func LatestUserInputIndex(messages []domainmessage.Message) int {
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.Role == domainmessage.RoleUser && (!message.IsSynthetic() || message.SyntheticReason == domainmessage.SyntheticReasonPlanStep || message.SyntheticReason == domainmessage.SyntheticReasonAutonomousPlanning) {
			return index
		}
	}
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == domainmessage.RoleUser && !IsDynamicContext(messages[index]) && messages[index].SyntheticReason != domainmessage.SyntheticReasonCompactionSummary {
			return index
		}
	}
	return -1
}

func (t TurnContext) Messages() []domainmessage.Message {
	messages := make([]domainmessage.Message, 0, 4)
	runtime := strings.TrimSpace(t.RuntimeInstruction)
	if runtime == domainmessage.RuntimeTurnBoundaryPrompt {
		// Previous builds stored this boilerplate even for plain Chat turns.
		runtime = ""
	}
	for _, message := range []domainmessage.Message{
		domainmessage.RuntimeInstruction(runtime), domainmessage.RAGContext(t.RAGContext),
		domainmessage.EnvironmentContext(t.EnvironmentContext),
	} {
		if message.IsSynthetic() {
			messages = append(messages, message)
		}
	}
	if text := strings.TrimSpace(t.PlanContext); text != "" {
		messages = append(messages, domainmessage.SyntheticUserText(domainmessage.SyntheticReasonPlanContext, text))
	}
	return messages
}

// requestHistory projects a raw suffix while keeping all checkpoint arithmetic
// against the raw transcript. The compatibility path keeps only state attached
// to the latest input; new request callers provide all current state explicitly.
func requestHistory(messages []domainmessage.Message, start int, turn *TurnContext) ([]domainmessage.Message, int) {
	latest := LatestUserInputIndex(messages)
	attached := latest
	for attached > 0 && IsDynamicContext(messages[attached-1]) {
		attached--
	}
	var currentContext []domainmessage.Message
	if turn != nil {
		currentContext = turn.Messages()
	} else if latest >= 0 {
		byReason := make(map[domainmessage.SyntheticReason]int)
		for index := max(0, attached); index < len(messages); index++ {
			if !IsDynamicContext(messages[index]) {
				continue
			}
			if text, ok := messages[index].RuntimeInstructionText(); ok && text == domainmessage.RuntimeTurnBoundaryPrompt {
				continue
			}
			message := domainmessage.Clone(messages[index])
			message.Role = domainmessage.RoleUser
			if previous, ok := byReason[message.SyntheticReason]; ok {
				currentContext[previous] = message
			} else {
				byReason[message.SyntheticReason] = len(currentContext)
				currentContext = append(currentContext, message)
			}
		}
	}
	result := make([]domainmessage.Message, 0, len(messages)-min(start, len(messages))+len(currentContext)+1)
	inserted := false
	// A checkpoint can end inside a long tool turn. Repeating its actual user
	// request preserves the active goal without replaying summarized tool calls.
	if latest >= 0 && latest < start {
		result = append(result, currentContext...)
		result = append(result, domainmessage.Clone(messages[latest]))
		inserted = true
	}
	filtered := 0
	for index := start; index < len(messages); index++ {
		message := messages[index]
		if IsDynamicContext(message) {
			filtered++
			continue
		}
		if index == latest {
			result = append(result, currentContext...)
			inserted = true
		}
		message = domainmessage.Clone(message)
		if message.Role == domainmessage.RoleSystem {
			// Only the fixed leading system message is provider-level policy.
			message.Role = domainmessage.RoleUser
		}
		result = append(result, message)
	}
	if !inserted {
		result = append(currentContext, result...)
	}
	return result, filtered
}

// ToolSafeChunks allows a cut between completed tool exchanges within a user
// turn. Never cut an assistant's parallel calls away from any of their results.
func ToolSafeChunks(messages []domainmessage.Message) [][]domainmessage.Message {
	var chunks [][]domainmessage.Message
	start := 0
	pending := make(map[string]bool)
	for index, message := range messages {
		for _, part := range message.Parts {
			if part.Type == domainmessage.PartToolCall && part.ToolCall != nil {
				pending[part.ToolCall.ID] = true
			}
			if part.Type == domainmessage.PartToolResult && part.ToolResult != nil {
				delete(pending, part.ToolResult.ToolCallID)
			}
		}
		if len(pending) != 0 {
			continue
		}
		// User/state fragments belong with the next assistant exchange.
		if index+1 < len(messages) && (message.Role == domainmessage.RoleUser || IsDynamicContext(message)) {
			continue
		}
		chunks = append(chunks, messages[start:index+1])
		start = index + 1
	}
	if start < len(messages) {
		chunks = append(chunks, messages[start:])
	}
	return chunks
}

func ValidateToolPairs(messages []domainmessage.Message) error {
	pending := make(map[string]bool)
	seen := make(map[string]bool)
	for _, message := range messages {
		if len(pending) > 0 && message.Role != domainmessage.RoleTool {
			return fmt.Errorf("incomplete tool exchange before %s message", message.Role)
		}
		for _, part := range message.Parts {
			if part.Type == domainmessage.PartToolCall && part.ToolCall != nil {
				id := part.ToolCall.ID
				if id == "" || seen[id] {
					return fmt.Errorf("empty or duplicate tool call id %q", id)
				}
				pending[id], seen[id] = true, true
			}
			if part.Type == domainmessage.PartToolResult && part.ToolResult != nil {
				id := part.ToolResult.ToolCallID
				if !pending[id] {
					return fmt.Errorf("orphan tool result %q", id)
				}
				delete(pending, id)
			}
		}
	}
	if len(pending) > 0 {
		return fmt.Errorf("incomplete tool exchange: %d result(s) missing", len(pending))
	}
	return nil
}
