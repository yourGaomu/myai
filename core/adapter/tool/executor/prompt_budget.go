package executor

import (
	generationcommand "myai/core/application/chat/generation/command"
	"myai/core/contextmgr"
	compaction "myai/core/domain/compaction"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	"myai/core/session"
)

// Compatibility callers have no actual tool schema. Reserve a conservative
// schema allowance and the configured output limit using the shared budget.
func promptToolResultBudget(current *session.Session, calls []domainmessage.ToolCall) int {
	if current == nil {
		return 0
	}
	window := contextmgr.ResolveWindowTokens(current.ContextWindowK, 0)
	output := 2048
	if current.GenerationSettings.MaxOutputTokens != nil {
		output = *current.GenerationSettings.MaxOutputTokens
	}
	budget := contextmgr.NewRequestBudget(window, 2048, output)
	snapshot := contextmgr.BuildSnapshotWithBudget(current.Messages, contextmgr.CheckpointForRequest(current.Messages, legacyCheckpoint(current)), current.CompactedMessages, current.ContextWindowK, budget.ReservedTokens())
	messages := append(domainmessage.CloneAll(snapshot.Messages), domainmessage.ToolCallMessage(calls))
	return budget.AvailableTokens(messages)
}

func promptToolResultBudgetForRequest(command generationcommand.ToolExecution, calls []domainmessage.ToolCall) int {
	if command.RequestBudget == nil {
		return promptToolResultBudget(command.Session, calls)
	}
	messages := domainmessage.CloneAll(command.PromptMessages)
	assistant := domainmessage.ToolCallMessage(calls)
	if command.AssistantMessage != nil {
		assistant = domainmessage.Assistant(command.AssistantMessage.Text(), command.AssistantMessage.Reasoning, calls)
	}
	messages = append(messages, assistant)
	return command.RequestBudget.AvailableTokens(messages)
}

func legacyCheckpoint(current *session.Session) compaction.Checkpoint {
	if current.CompactionCheckpoint != nil {
		return *current.CompactionCheckpoint.Clone()
	}
	return compaction.LegacyCheckpoint(current.Summary, current.CompactedMessages, current.CompactionSourceHash)
}

func limitToolResultsForPrompt(messages []domainmessage.Message, entries []domaintool.ExecutionEntry, budgetTokens int) ([]domainmessage.Message, []domaintool.ExecutionEntry) {
	limited, annotated, _ := limitToolResultsForRequest(messages, entries, budgetTokens)
	return limited, annotated
}

func limitToolResultsForRequest(messages []domainmessage.Message, entries []domaintool.ExecutionEntry, budgetTokens int) ([]domainmessage.Message, []domaintool.ExecutionEntry, error) {
	limited, budgetErr := contextmgr.LimitToolResults(messages, budgetTokens)
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
		entry.PromptContent, entry.PromptError = promptResult.Content, promptResult.ErrorMessage
		entry.PromptTruncated = true
	}
	return limited, annotatedEntries, budgetErr
}
