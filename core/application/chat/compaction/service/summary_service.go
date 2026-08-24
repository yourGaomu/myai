package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	compactionport "myai/core/application/chat/compaction/port"
	"myai/core/contextmgr"
	compaction "myai/core/domain/compaction"
	generation "myai/core/domain/generation"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
)

const (
	maxExistingSummaryTokens  = 600
	maxSummaryBatchTokens     = 1800
	maxGeneratedSummaryTokens = 768
)

type SummaryService struct{}

var _ compactionport.SummaryGenerator = SummaryService{}

func (SummaryService) Summarize(ctx context.Context, model modelport.ChatModelPort, existingSummary string, messages []domainmessage.Message) (string, error) {
	if model == nil {
		return "", errors.New("model is nil")
	}
	if strings.TrimSpace(messagesForSummary(messages)) == "" {
		return "", errors.New("no messages to compact")
	}
	existingSummary = existingSummaryForPrompt(existingSummary)
	batches := summaryBatches(messages, maxSummaryBatchTokens)
	summary := existingSummary
	for _, batch := range batches {
		text := messagesForSummary(batch)
		if strings.TrimSpace(text) == "" {
			continue
		}
		text = contextmgr.TruncateTextToTokens(text, maxSummaryBatchTokens)
		next, err := summarizeBatch(ctx, model, summary, text)
		if err != nil {
			return "", err
		}
		// JSON must remain structurally valid. Never truncate the generated
		// payload as plain text; the model output is validated in summarizeBatch.
		summary = next
	}
	if strings.TrimSpace(summary) == "" {
		return "", errors.New("compact summary is empty")
	}
	parsed, err := compaction.DecodeJSON(summary)
	if err != nil {
		return "", fmt.Errorf("compact summary is not valid JSON: %w", err)
	}
	encoded, err := compaction.EncodeJSON(parsed)
	if err != nil {
		return "", err
	}
	if contextmgr.EstimateTextTokens(encoded) > maxGeneratedSummaryTokens {
		return "", fmt.Errorf("compact summary exceeds %d tokens", maxGeneratedSummaryTokens)
	}
	return encoded, nil
}

func existingSummaryForPrompt(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if parsed, err := compaction.DecodeJSON(raw); err == nil {
		if encoded, encodeErr := compaction.EncodeJSON(parsed); encodeErr == nil {
			return encoded
		}
	}
	// Old sessions contain Markdown. Keep that compatibility path bounded for
	// the prompt; it is never persisted as a new structured checkpoint.
	return contextmgr.TruncateTextToTokens(raw, maxExistingSummaryTokens)
}

func summarizeBatch(ctx context.Context, model modelport.ChatModelPort, existingSummary string, text string) (string, error) {
	prompt := "Create a durable JSON context checkpoint for another coding-agent model that will continue this task.\n\n"
	prompt += "Return exactly one JSON object, with exactly these 11 fields and no Markdown, code fence, commentary, or trailing text:\n"
	prompt += "{\"current_goal\":\"\",\"preferences\":[],\"constraints\":[],\"decisions\":[],\"completed_work\":[],\"modified_files\":[],\"tool_verification\":[],\"problems\":[],\"open_tasks\":[],\"next_steps\":[],\"references\":[]}\n\n"
	prompt += "Rules:\n- Preserve exact file paths, commands, error messages, IDs, and configuration values when present.\n"
	prompt += "- current_goal must be a string; every other field must be an array of strings. Use an empty string or empty array when there is no evidence.\n"
	prompt += "- Do not invent facts.\n"
	prompt += "- Keep the newest status when old and new information conflict, and mention the conflict when it matters.\n"
	prompt += "- Keep tool calls paired with their tool results and retain failure status and verification evidence.\n"
	prompt += "- Do not include secrets, API keys, access tokens, passwords, or credentials.\n"
	prompt += "- Write in Chinese unless the source is mostly English.\n"
	if strings.TrimSpace(existingSummary) != "" {
		prompt += "\nExisting checkpoint to update:\n" + existingSummary
	}
	prompt += "\n\nComplete history batch to incorporate:\n" + text
	settings := generation.SystemDefaults()
	settings.Temperature = 0.2
	settings.MaxOutputTokens = maxGeneratedSummaryTokens
	generated, err := model.Generate(ctx, modelport.GenerateRequest{
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleSystem, "You are a context compression model for a coding assistant."),
			domainmessage.Text(domainmessage.RoleUser, prompt),
		},
		Settings: settings,
	})
	if err != nil {
		return "", err
	}
	summary := strings.TrimSpace(generated.Content)
	if summary == "" {
		return "", errors.New("compact summary is empty")
	}
	parsed, err := compaction.DecodeJSON(summary)
	if err != nil {
		return "", fmt.Errorf("model returned invalid compact summary JSON: %w", err)
	}
	encoded, err := compaction.EncodeJSON(parsed)
	if err != nil {
		return "", err
	}
	if contextmgr.EstimateTextTokens(encoded) > maxGeneratedSummaryTokens {
		return "", fmt.Errorf("model compact summary exceeds %d tokens", maxGeneratedSummaryTokens)
	}
	return encoded, nil
}

func summaryBatches(messages []domainmessage.Message, maxTokens int) [][]domainmessage.Message {
	chunks := contextmgr.MessageChunks(messages)
	batches := make([][]domainmessage.Message, 0, len(chunks))
	current := make([]domainmessage.Message, 0)
	currentTokens := 0
	for _, chunk := range chunks {
		chunkTokens := contextmgr.EstimateMessagesTokens(chunk)
		if len(current) > 0 && currentTokens+chunkTokens > maxTokens {
			batches = append(batches, current)
			current = make([]domainmessage.Message, 0)
			currentTokens = 0
		}
		current = append(current, chunk...)
		currentTokens += chunkTokens
	}
	if len(current) > 0 {
		batches = append(batches, current)
	}
	return batches
}

func messagesForSummary(messages []domainmessage.Message) string {
	var builder strings.Builder
	for _, message := range messages {
		if message.IsSynthetic() {
			continue
		}
		switch message.Role {
		case domainmessage.RoleSystem:
			continue
		case domainmessage.RoleUser:
			writeSummaryLine(&builder, "User", messageText(message, 0))
		case domainmessage.RoleAssistant:
			if message.HasToolCall() {
				writeSummaryLine(&builder, "Assistant tool call", messageText(message, 0))
			} else {
				writeSummaryLine(&builder, "Assistant", messageText(message, 0))
			}
		case domainmessage.RoleTool:
			writeSummaryLine(&builder, "Tool result", messageText(message, 0))
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
				content := part.ToolResult.Content
				if part.ToolResult.FullContent != "" {
					content = part.ToolResult.FullContent
				}
				errorMessage := part.ToolResult.ErrorMessage
				if part.ToolResult.FullErrorMessage != "" {
					errorMessage = part.ToolResult.FullErrorMessage
				}
				parts = append(parts, fmt.Sprintf("tool_result id=%s name=%s status=%s error_code=%s error=%s content=%s", part.ToolResult.ToolCallID, part.ToolResult.Name, part.ToolResult.Status, part.ToolResult.ErrorCode, errorMessage, content))
			}
		default:
			parts = append(parts, fmt.Sprint(part))
		}
	}
	return truncateForSummary(strings.Join(parts, "\n"), maxLength)
}
func truncateForSummary(text string, maxLength int) string {
	if maxLength <= 0 {
		return strings.TrimSpace(text)
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxLength {
		return string(runes)
	}
	return string(runes[:maxLength]) + "\n[truncated]"
}
