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
				messages = append(messages, messageWithRecord(domainmessage.SyntheticText(reason, record.Content), record))
			}
		case repository.RoleUser:
			message := domainmessage.Text(domainmessage.RoleUser, record.Content)
			if reason := domainmessage.SyntheticReason(record.SyntheticReason); reason != "" {
				message = domainmessage.SyntheticUserText(reason, record.Content)
			}
			messages = append(messages, messageWithRecord(message, record))
		case repository.RoleAssistant:
			messages = append(messages, messageWithRecord(domainmessage.Text(domainmessage.RoleAssistant, record.Content), record))
		case repository.RoleToolCall:
			calls := make([]domainmessage.ToolCall, 0, 1)
			recordIDs := make([]string, 0, 1)
			firstRecord := record
			for index < len(records) && records[index].Role == repository.RoleToolCall {
				callRecord := records[index]
				recordIDs = append(recordIDs, callRecord.ID)
				calls = append(calls, domainmessage.ToolCall{
					ID: callRecord.ToolCallID, Type: "function", Name: callRecord.ToolName, Arguments: callRecord.ToolArguments,
				})
				index++
			}
			index--
			message := messageWithRecord(domainmessage.ToolCallMessage(calls), firstRecord)
			message.RecordIDs = recordIDs
			messages = append(messages, message)
		case repository.RoleTool:
			content := record.Content
			errorMessage := record.ToolError
			if record.ToolPromptTruncated {
				content = record.ToolPromptContent
				errorMessage = record.ToolPromptError
			}
			message := domainmessage.ToolResultMessage(domainmessage.ToolResult{
				ToolCallID: record.ToolCallID, Name: record.ToolName, Content: content,
				Status: domaintool.ResultStatus(record.ToolStatus), ErrorCode: record.ToolErrorCode,
				ErrorMessage: errorMessage, Truncated: record.ToolTruncated,
				FullContent: record.Content, FullErrorMessage: record.ToolError,
				PromptTruncated: record.ToolPromptTruncated,
			})
			messages = append(messages, messageWithRecord(message, record))
		}
	}
	return messages
}

func messageWithRecord(message domainmessage.Message, record repository.MessageRecord) domainmessage.Message {
	message.ID = record.ID
	message.RecordIDs = []string{record.ID}
	message.Sequence = record.Sequence
	message.CreatedAt = record.CreatedAt
	return message
}
