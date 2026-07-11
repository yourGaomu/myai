package service

import (
	"strings"

	modelport "myai/core/port/model"
)

type ResponseCombiner struct{}

func (ResponseCombiner) Combine(current modelport.ChatResult, next modelport.ChatResult) modelport.ChatResult {
	current.Content = appendResponseText(current.Content, next.Content)
	current.Reasoning = appendResponseText(current.Reasoning, next.Reasoning)
	current.Usage = current.Usage.Add(next.Usage)
	current.ToolCalls = next.ToolCalls
	return current
}

func appendResponseText(existing string, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	if existing == "" {
		return next
	}
	if next == "" {
		return existing
	}
	return existing + "\n\n" + next
}
