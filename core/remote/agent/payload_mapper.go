package agent

import (
	"path/filepath"
	"strings"

	sessionresult "myai/core/application/session/result"
	"myai/core/llm"
	agentplan "myai/core/plan"
	"myai/core/remote/protocol"
	"myai/core/service"
	"myai/core/skill"
)

func contextInfoPayload(info service.ContextInfo) protocol.ContextInfo {
	return protocol.ContextInfo{
		WindowK:           info.WindowK,
		FullTokens:        info.FullTokens,
		SelectedTokens:    info.SelectedTokens,
		SummaryTokens:     info.SummaryTokens,
		PrefixTokens:      info.PrefixTokens,
		CacheableTokens:   info.CacheableTokens,
		FullMessages:      info.FullMessages,
		SelectedMessages:  info.SelectedMessages,
		CompactedMessages: info.CompactedMessages,
		HasSummary:        info.HasSummary,
		Truncated:         info.Truncated,
		SummaryVersion:    info.SummaryVersion,
		SummaryHash:       info.SummaryHash,
		PrefixHash:        info.PrefixHash,
	}
}

func compactInfoPayload(info service.CompactInfo) protocol.CompactInfo {
	return protocol.CompactInfo{
		Triggered:         info.Triggered,
		Reason:            info.Reason,
		BeforeTokens:      info.BeforeTokens,
		AfterTokens:       info.AfterTokens,
		NewMessages:       info.NewMessages,
		CompactedMessages: info.CompactedMessages,
		SummaryTokens:     info.SummaryTokens,
		SummaryVersion:    info.SummaryVersion,
		SummaryHash:       info.SummaryHash,
		PrefixHash:        info.PrefixHash,
		CacheableTokens:   info.CacheableTokens,
	}
}

func resolveSessionID(payloadSessionID string, messageSessionID string, currentSessionID string) string {
	sessionID := strings.TrimSpace(payloadSessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(messageSessionID)
	}
	if sessionID == "" {
		sessionID = strings.TrimSpace(currentSessionID)
	}
	return sessionID
}

func sessionSummaries(sessions []sessionresult.SessionListItem) []protocol.SessionSummary {
	summaries := make([]protocol.SessionSummary, 0, len(sessions))
	for _, session := range sessions {
		summaries = append(summaries, protocol.SessionSummary{
			ID:             session.ID,
			Title:          session.Title,
			Model:          session.Model,
			AgentMode:      agentModePayload(session.AgentMode),
			PermissionMode: session.PermissionMode,
			ContextWindowK: session.ContextWindowK,
			Usage:          tokenUsageResultToPayload(session.Usage),
			LastUsage:      tokenUsageResultToPayload(session.LastUsage),
			CurrentPlan:    planPayload(session.CurrentPlan),
			Deleted:        session.Deleted,
			DeletedAt:      session.DeletedAt,
			CreatedAt:      session.CreatedAt,
			UpdatedAt:      session.UpdatedAt,
		})
	}
	return summaries
}

func sessionHistoryMessages(records []sessionresult.MessageListItem) []protocol.SessionHistoryMessage {
	messages := make([]protocol.SessionHistoryMessage, 0, len(records))
	for _, record := range records {
		messages = append(messages, protocol.SessionHistoryMessage{
			ID:            record.ID,
			Role:          record.Role,
			Content:       record.Content,
			Reasoning:     record.Reasoning,
			ToolCallID:    record.ToolCallID,
			ToolName:      record.ToolName,
			ToolArguments: record.ToolArguments,
			ToolError:     record.ToolError,
			Usage:         tokenUsageRecordFromMessage(record),
			CreatedAt:     record.CreatedAt,
		})
	}
	return messages
}

func localHistoryUpToDate(local protocol.SessionHistoryMetaPayload, remote sessionresult.MessageHistoryMeta) bool {
	if int64(local.LocalMessageCount) != remote.MessageCount {
		return false
	}
	if strings.TrimSpace(local.LocalLastMessageID) != strings.TrimSpace(remote.LastMessageID) {
		return false
	}
	if local.LocalHistoryVersion != 0 && local.LocalHistoryVersion != remote.HistoryVersion {
		return false
	}
	return true
}

func tokenUsageRecordFromMessage(record sessionresult.MessageListItem) protocol.TokenUsage {
	return protocol.TokenUsage{
		PromptTokens:       record.PromptTokens,
		CompletionTokens:   record.CompletionTokens,
		TotalTokens:        record.TotalTokens,
		ReasoningTokens:    record.ReasoningTokens,
		PromptCachedTokens: record.PromptCachedTokens,
		Available: record.PromptTokens > 0 ||
			record.CompletionTokens > 0 ||
			record.TotalTokens > 0 ||
			record.ReasoningTokens > 0 ||
			record.PromptCachedTokens > 0,
	}
}

func tokenUsageResultToPayload(usage *sessionresult.TokenUsage) *protocol.TokenUsage {
	if usage == nil {
		return nil
	}
	payload := protocol.TokenUsage{
		PromptTokens:       usage.PromptTokens,
		CompletionTokens:   usage.CompletionTokens,
		TotalTokens:        usage.TotalTokens,
		ReasoningTokens:    usage.ReasoningTokens,
		PromptCachedTokens: usage.PromptCachedTokens,
		Available:          usage.Available,
	}
	if tokenUsagePayloadIsZero(payload) {
		return nil
	}
	return &payload
}

func tokenUsagePayloadPtr(usage llm.TokenUsage) *protocol.TokenUsage {
	payload := tokenUsagePayload(usage)
	if tokenUsagePayloadIsZero(payload) {
		return nil
	}
	return &payload
}

func tokenUsagePayloadIsZero(usage protocol.TokenUsage) bool {
	return !usage.Available &&
		usage.PromptTokens == 0 &&
		usage.CompletionTokens == 0 &&
		usage.TotalTokens == 0 &&
		usage.ReasoningTokens == 0 &&
		usage.PromptCachedTokens == 0
}

func planPayload(currentPlan *agentplan.Plan) *protocol.Plan {
	if currentPlan == nil {
		return nil
	}
	payload := &protocol.Plan{
		ID:         currentPlan.ID,
		SessionID:  currentPlan.SessionID,
		Goal:       currentPlan.Goal,
		Status:     currentPlan.Status,
		RawContent: currentPlan.RawContent,
		Steps:      make([]protocol.PlanStep, 0, len(currentPlan.Steps)),
		CreatedAt:  currentPlan.CreatedAt,
		UpdatedAt:  currentPlan.UpdatedAt,
	}
	for _, step := range currentPlan.Steps {
		payload.Steps = append(payload.Steps, protocol.PlanStep{
			ID:          step.ID,
			Order:       step.Order,
			Title:       step.Title,
			Description: step.Description,
			Status:      step.Status,
		})
	}
	return payload
}

func modelSummaries(models []llm.ModelInfo) []protocol.ModelSummary {
	summaries := make([]protocol.ModelSummary, 0, len(models))
	for _, model := range models {
		summaries = append(summaries, protocol.ModelSummary{
			ID:        model.ID,
			Name:      model.Name,
			Provider:  model.Provider,
			ModelName: model.ModelName,
			Enabled:   model.Enabled,
			IsDefault: model.IsDefault,
		})
	}
	return summaries
}

func skillSummaries(root string, skills []skill.Skill) []protocol.SkillSummary {
	summaries := make([]protocol.SkillSummary, 0, len(skills))
	for _, item := range skills {
		summaries = append(summaries, protocol.SkillSummary{
			Name:        item.Name,
			Description: item.Description,
			Path:        skillDisplayPath(root, item.Path),
			Triggers:    append([]string(nil), item.Triggers...),
			UpdatedAt:   item.UpdatedAt,
		})
	}
	return summaries
}

func assetSummaries(assets []sessionresult.AssetListItem) []protocol.AssetSummary {
	summaries := make([]protocol.AssetSummary, 0, len(assets))
	for _, asset := range assets {
		summaries = append(summaries, protocol.AssetSummary{
			ID:          asset.ID,
			SessionID:   asset.SessionID,
			RequestID:   asset.RequestID,
			ToolCallID:  asset.ToolCallID,
			ToolName:    asset.ToolName,
			Path:        asset.LocalPath,
			FileName:    asset.FileName,
			ContentType: asset.ContentType,
			Size:        asset.Size,
			ShortURL:    asset.ShortURL,
			Code:        asset.ShortCode,
			ExpiresAt:   asset.ExpiresAt,
			CreatedAt:   asset.CreatedAt,
		})
	}
	return summaries
}

func skillDisplayPath(root string, path string) string {
	root = strings.TrimSpace(root)
	path = strings.TrimSpace(path)
	if root != "" && path != "" {
		if rel, err := filepath.Rel(root, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}

func tokenUsagePayload(usage llm.TokenUsage) protocol.TokenUsage {
	return protocol.TokenUsage{
		PromptTokens:       usage.PromptTokens,
		CompletionTokens:   usage.CompletionTokens,
		TotalTokens:        usage.TotalTokens,
		ReasoningTokens:    usage.ReasoningTokens,
		PromptCachedTokens: usage.PromptCachedTokens,
		Available:          usage.Available,
	}
}

func findSessionSummary(sessions []protocol.SessionSummary, sessionID string) protocol.SessionSummary {
	for _, session := range sessions {
		if session.ID == sessionID {
			return session
		}
	}
	return protocol.SessionSummary{}
}

func agentModePayload(mode string) string {
	if strings.TrimSpace(mode) == "plan" {
		return "plan"
	}
	return "chat"
}
