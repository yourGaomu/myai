package langchaingo

import (
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/tmc/langchaingo/llms"

	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
)

// ToLLMRequest adds request-only image parts to their user messages. Models
// should call this only when their adapter speaks Chat Completions image_url.
func ToLLMRequest(request modelport.GenerateRequest) ([]llms.MessageContent, error) {
	messages := ToLLMS(request.Messages)
	if len(request.Images) > modelport.MaxInputImages {
		return nil, errors.New("model request exceeds the image count limit")
	}
	total := 0
	for _, image := range request.Images {
		if image.MessageIndex < 0 || image.MessageIndex >= len(messages) || request.Messages[image.MessageIndex].Role != domainmessage.RoleUser {
			return nil, errors.New("model request images must belong to user messages")
		}
		data := image.Bytes()
		if len(data) > modelport.MaxInputImageBytes || len(data) > modelport.MaxTotalInputImageBytes-total {
			return nil, errors.New("model request exceeds the image byte limit")
		}
		if (image.MIMEType != "image/jpeg" && image.MIMEType != "image/png") || http.DetectContentType(data) != image.MIMEType {
			return nil, errors.New("model request images must be JPEG or PNG")
		}
		total += len(data)
		url := "data:" + image.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(data)
		message := &messages[image.MessageIndex]
		message.Parts = append(message.Parts, llms.ImageURLPart(url))
	}
	return messages, nil
}

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
		Content:    result.PromptContent(),
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

func fromLLMToolCall(call llms.ToolCall) domainmessage.ToolCall {
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
		mapped = append(mapped, fromLLMToolCall(call))
	}
	return mapped
}
