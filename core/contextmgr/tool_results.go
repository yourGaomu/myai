package contextmgr

import (
	"fmt"
	"sort"

	domainmessage "myai/core/domain/message"
)

const MinimumToolPreviewTokens = 200

// LimitToolResults keeps small results intact and shares remaining capacity
// between large results. If even meaningful previews do not fit, return those
// previews and an explicit error; never manufacture empty successful results.
func LimitToolResults(messages []domainmessage.Message, budgetTokens int) ([]domainmessage.Message, error) {
	if EstimateMessagesTokens(messages) <= budgetTokens {
		return domainmessage.CloneAll(messages), nil
	}
	minimum := make([]domainmessage.Message, len(messages))
	for index, message := range messages {
		minimum[index] = limitedToolMessage(message, MinimumToolPreviewTokens)
	}
	minimumTokens := EstimateMessagesTokens(minimum)
	if minimumTokens > budgetTokens {
		return minimum, fmt.Errorf("tool result budget exhausted: meaningful previews need %d tokens, available=%d; compact before the next request", minimumTokens, budgetTokens)
	}
	type sizedMessage struct{ index, tokens int }
	order := make([]sizedMessage, len(messages))
	for index, message := range messages {
		order[index] = sizedMessage{index, EstimateMessagesTokens([]domainmessage.Message{message})}
	}
	sort.SliceStable(order, func(i, j int) bool { return order[i].tokens < order[j].tokens })
	limited := domainmessage.CloneAll(minimum)
	remaining := budgetTokens - minimumTokens
	for position, item := range order {
		index := item.index
		base := EstimateMessagesTokens([]domainmessage.Message{minimum[index]})
		allocation := base + remaining/(len(order)-position)
		if item.tokens <= allocation {
			limited[index] = domainmessage.Clone(messages[index])
		} else {
			low, high := MinimumToolPreviewTokens, allocation
			for low <= high {
				middle := low + (high-low)/2
				candidate := limitedToolMessage(messages[index], middle)
				if EstimateMessagesTokens([]domainmessage.Message{candidate}) <= allocation {
					limited[index] = candidate
					low = middle + 1
				} else {
					high = middle - 1
				}
			}
		}
		remaining -= EstimateMessagesTokens([]domainmessage.Message{limited[index]}) - base
	}
	if tokens := EstimateMessagesTokens(limited); tokens > budgetTokens {
		return limited, fmt.Errorf("tool result budget exhausted after serialization: need=%d, available=%d", tokens, budgetTokens)
	}
	return limited, nil
}

func limitedToolMessage(message domainmessage.Message, fieldBudget int) domainmessage.Message {
	limited := domainmessage.Clone(message)
	for index := range limited.Parts {
		part := &limited.Parts[index]
		if part.Type != domainmessage.PartToolResult || part.ToolResult == nil {
			continue
		}
		result := part.ToolResult
		contentBudget, errorBudget := fieldBudget, fieldBudget
		if result.Content != "" && result.ErrorMessage != "" {
			contentBudget, errorBudget = fieldBudget/2, fieldBudget-fieldBudget/2
		}
		content := TruncateTextToTokens(result.Content, contentBudget)
		errorText := TruncateTextToTokens(result.ErrorMessage, errorBudget)
		if content == result.Content && errorText == result.ErrorMessage {
			continue
		}
		if !result.PromptTruncated {
			result.FullContent, result.FullErrorMessage = result.Content, result.ErrorMessage
		}
		result.Content, result.ErrorMessage, result.PromptTruncated = content, errorText, true
	}
	return limited
}
