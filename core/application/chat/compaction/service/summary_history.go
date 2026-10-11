package service

import (
	"errors"
	"fmt"
	"strings"

	"myai/core/contextmgr"
	domainmessage "myai/core/domain/message"
)

type summaryEntry struct {
	header       string
	body         string
	continuation string
}

func summaryBatches(messages []domainmessage.Message, maxTokens int) ([]string, error) {
	if maxTokens <= 0 {
		return nil, errors.New("summary history budget must be positive")
	}
	var batches []string
	var batch strings.Builder
	flush := func() {
		if batch.Len() > 0 {
			batches = append(batches, batch.String())
			batch.Reset()
		}
	}
	appendText := func(text string) {
		if batch.Len() > 0 && !fitsSummaryBatch(batch.String()+text, maxTokens) {
			flush()
		}
		batch.WriteString(text)
	}
	// 1. 优先保持完整对话块和工具调用/结果配对，预算使用真正送入摘要的文本。
	for _, chunk := range contextmgr.MessageChunks(messages) {
		var entries []summaryEntry
		var text strings.Builder
		for _, message := range chunk {
			entry, ok := summaryEntryForMessage(message)
			if !ok {
				continue
			}
			entries = append(entries, entry)
			text.WriteString(entry.header + entry.body + "\n\n")
		}
		if text.Len() == 0 {
			continue
		}
		if fitsSummaryBatch(text.String(), maxTokens) {
			appendText(text.String())
			continue
		}
		// 2. 完整块超预算时才拆成消息片段；每段重复来源，正文按顺序完整覆盖。
		flush()
		for _, entry := range entries {
			parts, err := splitSummaryEntry(entry, maxTokens)
			if err != nil {
				return nil, err
			}
			for _, part := range parts {
				appendText(part)
			}
		}
	}
	flush()
	return batches, nil
}

func summaryEntryForMessage(message domainmessage.Message) (summaryEntry, bool) {
	// 1. 固定系统规则和临时提示不进入历史摘要；Synthetic 仅表示来源，不能一律删除。
	if message.Role == domainmessage.RoleSystem {
		return summaryEntry{}, false
	}
	label := ""
	if message.IsSynthetic() {
		switch message.SyntheticReason {
		case domainmessage.SyntheticReasonSubagentResult:
			label = "Subagent evidence"
		case domainmessage.SyntheticReasonInterAgentMessage:
			label = "Inter-agent evidence"
		case domainmessage.SyntheticReasonAutonomousPlanning:
			// 1.1 计划恢复消息包含真实失败信息；普通自动规划提示仍不进入摘要。
			if message.SourceKind == "plan_recovery" {
				label = "Plan recovery evidence"
			}
		}
	} else {
		switch message.Role {
		case domainmessage.RoleUser:
			label = "User"
		case domainmessage.RoleAssistant:
			label = "Assistant"
			if message.HasToolCall() {
				label = "Assistant tool call"
			}
		case domainmessage.RoleTool:
			label = "Tool result"
		}
	}
	if label == "" {
		return summaryEntry{}, false
	}
	body := messageText(message)
	if body == "" {
		return summaryEntry{}, false
	}
	header := label + ":\n"
	// 2. 保留结构化来源并转义字段，避免报告被误标为用户要求或关联到另一任务。
	if message.IsSynthetic() || message.SourceID != "" || message.SourceKind != "" || message.SourceTaskID != "" {
		header += fmt.Sprintf("source_reason=%q source_id=%q source_kind=%q source_task_id=%q\n", message.SyntheticReason, message.SourceID, message.SourceKind, message.SourceTaskID)
	}
	// 2.1 大工具结果分段后仍携带调用身份，后续片段可以关联之前的调用和失败状态。
	var continuation strings.Builder
	for _, part := range message.Parts {
		if call := part.ToolCall; part.Type == domainmessage.PartToolCall && call != nil {
			fmt.Fprintf(&continuation, "tool_call_id=%q tool_name=%q\n", call.ID, call.Name)
		}
		if result := part.ToolResult; part.Type == domainmessage.PartToolResult && result != nil {
			fmt.Fprintf(&continuation, "tool_call_id=%q tool_name=%q status=%q error_code=%q\n", result.ToolCallID, result.Name, result.Status, result.ErrorCode)
		}
	}
	return summaryEntry{header: header, body: body, continuation: continuation.String()}, true
}

func splitSummaryEntry(entry summaryEntry, maxTokens int) ([]string, error) {
	full := entry.header + entry.body + "\n\n"
	if fitsSummaryBatch(full, maxTokens) {
		return []string{full}, nil
	}
	var parts []string
	runes := []rune(entry.body)
	for segment := 1; len(runes) > 0; segment++ {
		header := entry.header + entry.continuation + fmt.Sprintf("[segment %d of the same history entry]\n", segment)
		// 3. 按 UTF-8 字符边界寻找可容纳的最长前缀；只移动游标，不删除中间内容。
		low, high := 0, min(len(runes), maxTokens*4)
		for low < high {
			middle := low + (high-low+1)/2
			if fitsSummaryBatch(header+string(runes[:middle])+"\n\n", maxTokens) {
				low = middle
			} else {
				high = middle - 1
			}
		}
		if low == 0 {
			return nil, errors.New("summary source metadata exceeds history budget")
		}
		parts = append(parts, header+string(runes[:low])+"\n\n")
		runes = runes[low:]
	}
	return parts, nil
}

func fitsSummaryBatch(text string, maxTokens int) bool {
	// 4. token 估算忽略空白，额外限制字节数，防止大量空白绕过请求大小限制。
	return len(text) <= maxTokens*4 && contextmgr.EstimateTextTokens(text) <= maxTokens
}
