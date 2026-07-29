package executor

import (
	"strings"

	"myai/core/contextmgr"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	"myai/core/session"
)

const minimumReservedResponseTokens = 1024

func promptToolResultBudget(current *session.Session, calls []domainmessage.ToolCall) int {
	if current == nil {
		return 0
	}
	windowTokens := contextmgr.NormalizeWindowK(current.ContextWindowK) * 1000
	baseTokens := 0
	if len(current.Messages) > 0 && current.Messages[0].Role == domainmessage.RoleSystem {
		baseTokens += contextmgr.EstimateMessagesTokens(current.Messages[:1])
	}
	if summary := strings.TrimSpace(current.Summary); summary != "" {
		summaryTokens := contextmgr.EstimateTextTokens(summary)
		if summaryTokens > 1024 {
			summaryTokens = 1024
		}
		baseTokens += summaryTokens + 4
	}
	if current.CurrentPlan != nil {
		baseTokens += 512
	}

	callTokens := 0
	if len(calls) > 0 {
		callTokens = contextmgr.EstimateMessagesTokens([]domainmessage.Message{domainmessage.ToolCallMessage(calls)})
	}
	reservedResponseTokens := windowTokens / 8
	if reservedResponseTokens < minimumReservedResponseTokens {
		reservedResponseTokens = minimumReservedResponseTokens
	}

	available := windowTokens - baseTokens - contextmgr.CurrentTurnTokens(current.Messages) - callTokens - reservedResponseTokens
	maximumBatch := windowTokens / 2
	if available > maximumBatch {
		available = maximumBatch
	}
	if available < 0 {
		return 0
	}
	return available
}

func limitToolResultsForPrompt(messages []domainmessage.Message, entries []domaintool.ExecutionEntry, budgetTokens int) ([]domainmessage.Message, []domaintool.ExecutionEntry) {
	if contextmgr.EstimateMessagesTokens(messages) <= budgetTokens {
		return messages, entries
	}

	maximumFieldTokens := 0
	for _, message := range messages {
		for _, part := range message.Parts {
			if part.Type != domainmessage.PartToolResult || part.ToolResult == nil {
				continue
			}
			contentTokens := contextmgr.EstimateTextTokens(part.ToolResult.Content)
			errorTokens := contextmgr.EstimateTextTokens(part.ToolResult.ErrorMessage)
			if contentTokens > maximumFieldTokens {
				maximumFieldTokens = contentTokens
			}
			if errorTokens > maximumFieldTokens {
				maximumFieldTokens = errorTokens
			}
		}
	}

	low, high := 0, maximumFieldTokens
	limited := truncateToolResultFields(messages, 0)
	for low <= high {
		middle := (low + high) / 2
		candidate := truncateToolResultFields(messages, middle)
		if contextmgr.EstimateMessagesTokens(candidate) <= budgetTokens {
			limited = candidate
			low = middle + 1
		} else {
			high = middle - 1
		}
	}

	promptResults := make(map[string]domainmessage.ToolResult)
	for _, message := range limited {
		if result, ok := message.FirstToolResult(); ok {
			promptResults[result.ToolCallID] = result
		}
	}
	annotatedEntries := append([]domaintool.ExecutionEntry(nil), entries...)
	for index := range annotatedEntries {
		entry := &annotatedEntries[index]
		if entry.Kind != domaintool.ExecutionEntryToolResult {
			continue
		}
		promptResult, ok := promptResults[entry.ToolCallID]
		if !ok || promptResult.Content == entry.Content && promptResult.ErrorMessage == entry.Error {
			continue
		}
		entry.PromptContent = promptResult.Content
		entry.PromptError = promptResult.ErrorMessage
		entry.PromptTruncated = true
	}
	return limited, annotatedEntries
}

func truncateToolResultFields(messages []domainmessage.Message, maxFieldTokens int) []domainmessage.Message {
	limited := domainmessage.CloneAll(messages)
	for messageIndex := range limited {
		for partIndex := range limited[messageIndex].Parts {
			part := &limited[messageIndex].Parts[partIndex]
			if part.Type != domainmessage.PartToolResult || part.ToolResult == nil {
				continue
			}
			content := contextmgr.TruncateTextToTokens(part.ToolResult.Content, maxFieldTokens)
			errorMessage := contextmgr.TruncateTextToTokens(part.ToolResult.ErrorMessage, maxFieldTokens)
			if content != part.ToolResult.Content || errorMessage != part.ToolResult.ErrorMessage {
				if !part.ToolResult.PromptTruncated {
					part.ToolResult.FullContent = part.ToolResult.Content
					part.ToolResult.FullErrorMessage = part.ToolResult.ErrorMessage
				}
				part.ToolResult.PromptTruncated = true
			}
			part.ToolResult.Content = content
			part.ToolResult.ErrorMessage = errorMessage
		}
	}
	return limited
}
