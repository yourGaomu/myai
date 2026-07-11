package langchaingo

import (
	"github.com/tmc/langchaingo/llms"

	domainmessage "myai/core/domain/message"
)

func ToLLMS(messages []domainmessage.Message) []llms.MessageContent {
	if len(messages) == 0 {
		return nil
	}
	mapped := make([]llms.MessageContent, 0, len(messages))
	for _, item := range messages {
		mapped = append(mapped, ToLLMMessage(item))
	}
	return mapped
}

func ToLLMMessage(message domainmessage.Message) llms.MessageContent {
	return llms.MessageContent{
		Role:  ToLLMRole(message.Role),
		Parts: ToLLMParts(message.Parts),
	}
}

func ToLLMParts(parts []domainmessage.Part) []llms.ContentPart {
	mapped := make([]llms.ContentPart, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case domainmessage.PartText:
			mapped = append(mapped, llms.TextContent{Text: part.Text})
		case domainmessage.PartToolCall:
			if part.ToolCall != nil {
				mapped = append(mapped, ToLLMToolCall(*part.ToolCall))
			}
		case domainmessage.PartToolResult:
			if part.ToolResult != nil {
				mapped = append(mapped, ToLLMToolCallResponse(*part.ToolResult))
			}
		}
	}
	return mapped
}

func ToLLMToolCall(call domainmessage.ToolCall) llms.ToolCall {
	callType := call.Type
	if callType == "" {
		callType = "function"
	}
	return llms.ToolCall{
		ID:   call.ID,
		Type: callType,
		FunctionCall: &llms.FunctionCall{
			Name:      call.Name,
			Arguments: call.Arguments,
		},
	}
}

func ToLLMToolCallResponse(result domainmessage.ToolResult) llms.ToolCallResponse {
	return llms.ToolCallResponse{
		ToolCallID: result.ToolCallID,
		Name:       result.Name,
		Content:    result.Content,
	}
}

func ToLLMRole(role domainmessage.Role) llms.ChatMessageType {
	switch role {
	case domainmessage.RoleSystem:
		return llms.ChatMessageTypeSystem
	case domainmessage.RoleUser:
		return llms.ChatMessageTypeHuman
	case domainmessage.RoleAssistant:
		return llms.ChatMessageTypeAI
	case domainmessage.RoleTool:
		return llms.ChatMessageTypeTool
	default:
		return llms.ChatMessageTypeGeneric
	}
}

func FromLLMS(messages []llms.MessageContent) []domainmessage.Message {
	if len(messages) == 0 {
		return nil
	}
	mapped := make([]domainmessage.Message, 0, len(messages))
	for _, item := range messages {
		mapped = append(mapped, FromLLMMessage(item))
	}
	return mapped
}

func FromLLMMessage(message llms.MessageContent) domainmessage.Message {
	return domainmessage.Message{
		Role:  FromLLMRole(message.Role),
		Parts: FromLLMParts(message.Parts),
	}
}

func FromLLMParts(parts []llms.ContentPart) []domainmessage.Part {
	mapped := make([]domainmessage.Part, 0, len(parts))
	for _, part := range parts {
		switch value := part.(type) {
		case llms.TextContent:
			mapped = append(mapped, domainmessage.Part{Type: domainmessage.PartText, Text: value.Text})
		case llms.ToolCall:
			call := FromLLMToolCall(value)
			mapped = append(mapped, domainmessage.Part{Type: domainmessage.PartToolCall, ToolCall: &call})
		case llms.ToolCallResponse:
			result := FromLLMToolCallResponse(value)
			mapped = append(mapped, domainmessage.Part{Type: domainmessage.PartToolResult, ToolResult: &result})
		}
	}
	return mapped
}

func FromLLMToolCall(call llms.ToolCall) domainmessage.ToolCall {
	mapped := domainmessage.ToolCall{
		ID:   call.ID,
		Type: call.Type,
	}
	if call.FunctionCall != nil {
		mapped.Name = call.FunctionCall.Name
		mapped.Arguments = call.FunctionCall.Arguments
	}
	return mapped
}

func FromLLMToolCalls(calls []llms.ToolCall) []domainmessage.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	mapped := make([]domainmessage.ToolCall, 0, len(calls))
	for _, call := range calls {
		mapped = append(mapped, FromLLMToolCall(call))
	}
	return mapped
}

func FromLLMToolCallResponse(response llms.ToolCallResponse) domainmessage.ToolResult {
	return domainmessage.ToolResult{
		ToolCallID: response.ToolCallID,
		Name:       response.Name,
		Content:    response.Content,
	}
}

func FromLLMRole(role llms.ChatMessageType) domainmessage.Role {
	switch role {
	case llms.ChatMessageTypeSystem:
		return domainmessage.RoleSystem
	case llms.ChatMessageTypeHuman:
		return domainmessage.RoleUser
	case llms.ChatMessageTypeAI:
		return domainmessage.RoleAssistant
	case llms.ChatMessageTypeTool:
		return domainmessage.RoleTool
	default:
		return domainmessage.RoleUser
	}
}
