package service

import (
	domainmessage "myai/core/domain/message"
	"myai/core/llm"
	repository "myai/core/port/repository"
	"myai/core/session"
)

func TokenUsageFromRecord(record *repository.TokenUsageRecord) llm.TokenUsage {
	if record == nil {
		return llm.TokenUsage{}
	}
	return llm.TokenUsage{
		PromptTokens: record.PromptTokens, CompletionTokens: record.CompletionTokens,
		TotalTokens: record.TotalTokens, ReasoningTokens: record.ReasoningTokens,
		PromptCachedTokens: record.PromptCachedTokens, Available: record.Available,
	}
}

func MessagesFromRecords(records []repository.MessageRecord) []domainmessage.Message {
	messages := make([]domainmessage.Message, 0, len(records)+1)
	messages = append(messages, domainmessage.Text(domainmessage.RoleSystem, session.SystemPrompt()))
	for _, record := range records {
		switch record.Role {
		case repository.RoleUser:
			messages = append(messages, domainmessage.Text(domainmessage.RoleUser, record.Content))
		case repository.RoleAssistant:
			messages = append(messages, domainmessage.Text(domainmessage.RoleAssistant, record.Content))
		case repository.RoleToolCall:
			messages = append(messages, domainmessage.ToolCallMessage([]domainmessage.ToolCall{{
				ID: record.ToolCallID, Type: "function", Name: record.ToolName, Arguments: record.ToolArguments,
			}}))
		case repository.RoleTool:
			messages = append(messages, domainmessage.ToolResultMessage(domainmessage.ToolResult{
				ToolCallID: record.ToolCallID, Name: record.ToolName, Content: record.Content,
			}))
		}
	}
	return messages
}
