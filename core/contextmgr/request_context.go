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
		if message.Role == domainmessage.RoleUser && (!message.IsSynthetic() || isSyntheticInput(message.SyntheticReason)) {
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

func isSyntheticInput(reason domainmessage.SyntheticReason) bool {
	switch reason {
	case domainmessage.SyntheticReasonPlanStep, domainmessage.SyntheticReasonAutonomousPlanning,
		domainmessage.SyntheticReasonSubagentResult, domainmessage.SyntheticReasonInterAgentMessage:
		return true
	default:
		return false
	}
}

func (t TurnContext) Messages() []domainmessage.Message {
	messages := make([]domainmessage.Message, 0, 4)
	runtime := strings.TrimSpace(t.RuntimeInstruction)
	if runtime == domainmessage.RuntimeTurnBoundaryPrompt {
		// Previous builds stored this boilerplate even for plain Chat turns.
		runtime = ""
	}
	runtimeMessage := domainmessage.RuntimeInstruction(runtime)
	runtimeMessage.SkillPromptTokens = t.SkillPromptTokens
	for _, message := range []domainmessage.Message{
		runtimeMessage, domainmessage.RAGContext(t.RAGContext),
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
// against the raw transcript. Historical dynamic context stays in place; only
// context attached to the active input may be refreshed, and new context events
// remain at their appended positions.
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
	filtered := 0
	inserted := false
	if latest >= 0 && latest < start {
		// The checkpoint cut through the active turn. Restore its input and any
		// context not already present in the retained tool tail.
		for _, current := range currentContext {
			if !containsRequestContext(messages[start:], current) {
				result = append(result, domainmessage.Clone(current))
			}
		}
		result = append(result, cloneRequestMessage(messages[latest]))
		inserted = true
	}

	attachedReplacements := make(map[int]domainmessage.Message)
	attachedSkips := make(map[int]bool)
	missingContext := make([]domainmessage.Message, 0, len(currentContext))
	if latest >= start && latest >= 0 {
		byReason := make(map[domainmessage.SyntheticReason]domainmessage.Message, len(currentContext))
		for _, message := range currentContext {
			byReason[message.SyntheticReason] = message
		}
		used := make(map[domainmessage.SyntheticReason]bool, len(currentContext))
		for index := max(start, attached); index < latest; index++ {
			message := messages[index]
			if !IsDynamicContext(message) {
				continue
			}
			current, exists := byReason[message.SyntheticReason]
			if !exists || containsRequestContext(messages[latest:], current) {
				continue
			}
			if used[message.SyntheticReason] {
				attachedSkips[index] = true
				filtered++
				continue
			}
			used[message.SyntheticReason] = true
			attachedReplacements[index] = current
			if !sameRequestContext(message, current) {
				filtered++
			}
		}

		// A current context already recorded elsewhere in the retained suffix
		// is an append-only event; do not move it back in front of the input.
		for _, current := range currentContext {
			if used[current.SyntheticReason] || containsRequestContext(messages[latest:], current) {
				continue
			}
			missingContext = append(missingContext, domainmessage.Clone(current))
		}
	}
	for index := start; index < len(messages); index++ {
		if attachedSkips[index] {
			continue
		}
		if index == latest && !inserted {
			if latest < 0 {
				result = append(result, cloneRequestContext(currentContext)...)
			} else if latest < start {
				// Already restored above.
			} else {
				// Missing request-only context is inserted at the current input
				// boundary. Production callers persist plan/environment events
				// before snapshotting, so later samples replay them in place.
				result = append(result, missingContext...)
			}
			inserted = true
		}
		if replacement, ok := attachedReplacements[index]; ok {
			result = append(result, domainmessage.Clone(replacement))
			continue
		}
		result = append(result, cloneRequestMessage(messages[index]))
	}
	if !inserted {
		result = append(cloneRequestContext(currentContext), result...)
	}
	return result, filtered
}

func cloneRequestContext(messages []domainmessage.Message) []domainmessage.Message {
	cloned := make([]domainmessage.Message, 0, len(messages))
	for _, message := range messages {
		message = domainmessage.Clone(message)
		message.Role = domainmessage.RoleUser
		cloned = append(cloned, message)
	}
	return cloned
}

func cloneRequestMessage(message domainmessage.Message) domainmessage.Message {
	message = domainmessage.Clone(message)
	if message.Role == domainmessage.RoleSystem {
		// Only the fixed leading system message is provider-level policy.
		message.Role = domainmessage.RoleUser
	}
	return message
}

func sameRequestContext(left, right domainmessage.Message) bool {
	return left.SyntheticReason == right.SyntheticReason && left.Text() == right.Text()
}

func containsRequestContext(messages []domainmessage.Message, context domainmessage.Message) bool {
	for _, message := range messages {
		if IsDynamicContext(message) && sameRequestContext(message, context) {
			return true
		}
	}
	return false
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
