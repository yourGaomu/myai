package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	compactionport "myai/core/application/chat/compaction/port"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
)

type SummaryService struct{}

var _ compactionport.SummaryGenerator = SummaryService{}

func (SummaryService) Summarize(ctx context.Context, model modelport.ChatModelPort, existingSummary string, messages []domainmessage.Message) (string, error) {
	if model == nil {
		return "", errors.New("model is nil")
	}
	text := messagesForSummary(messages)
	if strings.TrimSpace(text) == "" {
		return "", errors.New("no messages to compact")
	}
	prompt := "Compress the conversation history for a local coding agent.\n\nKeep durable information only:\n- User goals and preferences.\n- Architecture and implementation decisions.\n- Important files, tools, permissions, and configuration.\n- Completed work and verification results.\n- Open tasks, blockers, and next steps.\n- Any safety constraints or user instructions.\n\nDo not include secrets, API keys, or credentials.\nWrite a concise but useful summary in Chinese unless the source content is mostly English."
	if strings.TrimSpace(existingSummary) != "" {
		prompt += "\n\nExisting summary:\n" + existingSummary
	}
	prompt += "\n\nNew history to compact:\n" + text
	generated, err := model.Generate(ctx, modelport.GenerateRequest{Messages: []domainmessage.Message{domainmessage.Text(domainmessage.RoleSystem, "You are a context compression model for a coding assistant."), domainmessage.Text(domainmessage.RoleUser, prompt)}})
	if err != nil {
		return "", err
	}
	summary := strings.TrimSpace(generated.Content)
	if summary == "" {
		return "", errors.New("compact summary is empty")
	}
	return summary, nil
}

func messagesForSummary(messages []domainmessage.Message) string {
	var builder strings.Builder
	for _, message := range messages {
		switch message.Role {
		case domainmessage.RoleSystem:
			continue
		case domainmessage.RoleUser:
			writeSummaryLine(&builder, "User", messageText(message, 4000))
		case domainmessage.RoleAssistant:
			if message.HasToolCall() {
				writeSummaryLine(&builder, "Assistant tool call", messageText(message, 2000))
			} else {
				writeSummaryLine(&builder, "Assistant", messageText(message, 4000))
			}
		case domainmessage.RoleTool:
			writeSummaryLine(&builder, "Tool result", messageText(message, 2000))
		}
	}
	return builder.String()
}

func writeSummaryLine(builder *strings.Builder, role, text string) {
	text = strings.TrimSpace(text)
	if text != "" {
		builder.WriteString(role + ":\n" + text + "\n\n")
	}
}
func messageText(message domainmessage.Message, maxLength int) string {
	parts := make([]string, 0, len(message.Parts))
	for _, part := range message.Parts {
		switch part.Type {
		case domainmessage.PartText:
			parts = append(parts, part.Text)
		case domainmessage.PartToolCall:
			if part.ToolCall == nil {
				parts = append(parts, "tool_call")
			} else {
				parts = append(parts, fmt.Sprintf("tool_call id=%s name=%s args=%s", part.ToolCall.ID, part.ToolCall.Name, part.ToolCall.Arguments))
			}
		case domainmessage.PartToolResult:
			if part.ToolResult == nil {
				parts = append(parts, "tool_result")
			} else {
				parts = append(parts, fmt.Sprintf("tool_result id=%s name=%s content=%s", part.ToolResult.ToolCallID, part.ToolResult.Name, part.ToolResult.Content))
			}
		default:
			parts = append(parts, fmt.Sprint(part))
		}
	}
	return truncateForSummary(strings.Join(parts, "\n"), maxLength)
}
func truncateForSummary(text string, maxLength int) string {
	if maxLength <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxLength {
		return string(runes)
	}
	return string(runes[:maxLength]) + "\n[truncated]"
}
