package service

import (
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
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

func MessagesFromRecords(records []repository.MessageRecord, systemInstruction ...string) []domainmessage.Message {
	messages := make([]domainmessage.Message, 0, len(records)+1)
	instruction := ""
	if len(systemInstruction) > 0 {
		instruction = systemInstruction[0]
	}
	messages = append(messages, domainmessage.Text(domainmessage.RoleSystem, session.SystemPromptWithInstruction(instruction)))
	for index := 0; index < len(records); index++ {
		record := records[index]
		switch record.Role {
		case repository.RoleSystem:
			reason := domainmessage.SyntheticReason(record.SyntheticReason)
			if reason != "" {
				messages = append(messages, domainmessage.SyntheticText(reason, record.Content))
			}
		case repository.RoleUser:
			messages = append(messages, domainmessage.Text(domainmessage.RoleUser, record.Content))
		case repository.RoleAssistant:
			messages = append(messages, domainmessage.Text(domainmessage.RoleAssistant, record.Content))
		case repository.RoleToolCall:
			calls := make([]domainmessage.ToolCall, 0, 1)
			for index < len(records) && records[index].Role == repository.RoleToolCall {
				callRecord := records[index]
				calls = append(calls, domainmessage.ToolCall{
					ID: callRecord.ToolCallID, Type: "function", Name: callRecord.ToolName, Arguments: callRecord.ToolArguments,
				})
				index++
			}
			index--
			messages = append(messages, domainmessage.ToolCallMessage(calls))
		case repository.RoleTool:
			messages = append(messages, domainmessage.ToolResultMessage(domainmessage.ToolResult{
				ToolCallID: record.ToolCallID, Name: record.ToolName, Content: record.Content,
				Status: domaintool.ResultStatus(record.ToolStatus), ErrorCode: record.ToolErrorCode,
				ErrorMessage: record.ToolError, Truncated: record.ToolTruncated,
			}))
		}
	}
	return messages
}
