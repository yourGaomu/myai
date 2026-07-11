package service

import (
	sessionresult "myai/core/application/session/result"
	repository "myai/core/port/repository"
)

func SessionListItems(records []repository.SessionRecord) []sessionresult.SessionListItem {
	items := make([]sessionresult.SessionListItem, 0, len(records))
	for _, record := range records {
		items = append(items, sessionresult.SessionListItem{
			ID: record.ID, Title: record.Title, Model: record.Model,
			AgentMode: record.AgentMode, PermissionMode: record.PermissionMode,
			ContextWindowK: record.ContextWindowK, Usage: TokenUsageResultFromRecord(record.Usage),
			LastUsage: TokenUsageResultFromRecord(record.LastUsage), CurrentPlan: record.CurrentPlan,
			Deleted: record.Deleted, DeletedAt: record.DeletedAt,
			CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		})
	}
	return items
}

func MessageListItems(records []repository.MessageRecord) []sessionresult.MessageListItem {
	items := make([]sessionresult.MessageListItem, 0, len(records))
	for _, record := range records {
		items = append(items, sessionresult.MessageListItem{
			ID: record.ID, Role: record.Role, Content: record.Content, Reasoning: record.Reasoning,
			ToolCallID: record.ToolCallID, ToolName: record.ToolName, ToolArguments: record.ToolArguments,
			ToolError: record.ToolError, PromptTokens: record.PromptTokens,
			CompletionTokens: record.CompletionTokens, TotalTokens: record.TotalTokens,
			ReasoningTokens: record.ReasoningTokens, PromptCachedTokens: record.PromptCachedTokens,
			CreatedAt: record.CreatedAt,
		})
	}
	return items
}

func AssetListItems(records []repository.AssetRecord) []sessionresult.AssetListItem {
	items := make([]sessionresult.AssetListItem, 0, len(records))
	for _, record := range records {
		items = append(items, sessionresult.AssetListItem{
			ID: record.ID, SessionID: record.SessionID, RequestID: record.RequestID,
			ToolCallID: record.ToolCallID, ToolName: record.ToolName, LocalPath: record.LocalPath,
			FileName: record.FileName, ContentType: record.ContentType, Size: record.Size,
			ShortURL: record.ShortURL, ShortCode: record.ShortCode,
			ExpiresAt: record.ExpiresAt, CreatedAt: record.CreatedAt,
		})
	}
	return items
}

func MessageHistoryMetaResultFromRecord(record repository.MessageHistoryMeta) sessionresult.MessageHistoryMeta {
	return sessionresult.MessageHistoryMeta{
		SessionID: record.SessionID, MessageCount: record.MessageCount,
		LastMessageID: record.LastMessageID, LastMessageCreatedAt: record.LastMessageCreatedAt,
		HistoryVersion: record.HistoryVersion,
	}
}

func TokenUsageResultFromRecord(record *repository.TokenUsageRecord) *sessionresult.TokenUsage {
	if record == nil {
		return nil
	}
	return &sessionresult.TokenUsage{
		PromptTokens: record.PromptTokens, CompletionTokens: record.CompletionTokens,
		TotalTokens: record.TotalTokens, ReasoningTokens: record.ReasoningTokens,
		PromptCachedTokens: record.PromptCachedTokens, Available: record.Available,
	}
}
