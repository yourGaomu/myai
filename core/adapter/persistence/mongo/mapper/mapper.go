package mapper

import (
	"myai/core/adapter/persistence/mongo/po"
	domainmodel "myai/core/domain/model"
	agentplan "myai/core/plan"
	repository "myai/core/port/repository"
)

func SessionDocumentFromRecord(record repository.SessionRecord) po.SessionDocument {
	// mapper 是 BSON PO 与仓库 Record 的唯一转换边界，内层对象不携带 bson 标签。
	return po.SessionDocument{
		ID:                record.ID,
		Model:             record.Model,
		AgentMode:         record.AgentMode,
		PermissionMode:    record.PermissionMode,
		ContextWindowK:    record.ContextWindowK,
		Summary:           record.Summary,
		CompactedMessages: record.CompactedMessages,
		CompactedAt:       record.CompactedAt,
		Title:             record.Title,
		Usage:             TokenUsageDocumentFromRecord(record.Usage),
		LastUsage:         TokenUsageDocumentFromRecord(record.LastUsage),
		CurrentPlan:       PlanDocumentFromDomain(record.CurrentPlan),
		Deleted:           record.Deleted,
		DeletedAt:         record.DeletedAt,
		CreatedAt:         record.CreatedAt,
		UpdatedAt:         record.UpdatedAt,
	}
}

func SessionRecordFromDocument(document po.SessionDocument) repository.SessionRecord {
	return repository.SessionRecord{
		ID:                document.ID,
		Model:             document.Model,
		AgentMode:         document.AgentMode,
		PermissionMode:    document.PermissionMode,
		ContextWindowK:    document.ContextWindowK,
		Summary:           document.Summary,
		CompactedMessages: document.CompactedMessages,
		CompactedAt:       document.CompactedAt,
		Title:             document.Title,
		Usage:             TokenUsageRecordFromDocument(document.Usage),
		LastUsage:         TokenUsageRecordFromDocument(document.LastUsage),
		CurrentPlan:       PlanDomainFromDocument(document.CurrentPlan),
		Deleted:           document.Deleted,
		DeletedAt:         document.DeletedAt,
		CreatedAt:         document.CreatedAt,
		UpdatedAt:         document.UpdatedAt,
	}
}

func MessageDocumentFromRecord(record repository.MessageRecord) po.MessageDocument {
	return po.MessageDocument{
		ID:                 record.ID,
		SessionID:          record.SessionID,
		Role:               record.Role,
		Content:            record.Content,
		Reasoning:          record.Reasoning,
		ToolCallID:         record.ToolCallID,
		ToolName:           record.ToolName,
		ToolArguments:      record.ToolArguments,
		ToolError:          record.ToolError,
		PromptTokens:       record.PromptTokens,
		CompletionTokens:   record.CompletionTokens,
		TotalTokens:        record.TotalTokens,
		ReasoningTokens:    record.ReasoningTokens,
		PromptCachedTokens: record.PromptCachedTokens,
		CreatedAt:          record.CreatedAt,
	}
}

func MessageRecordFromDocument(document po.MessageDocument) repository.MessageRecord {
	return repository.MessageRecord{
		ID:                 document.ID,
		SessionID:          document.SessionID,
		Role:               document.Role,
		Content:            document.Content,
		Reasoning:          document.Reasoning,
		ToolCallID:         document.ToolCallID,
		ToolName:           document.ToolName,
		ToolArguments:      document.ToolArguments,
		ToolError:          document.ToolError,
		PromptTokens:       document.PromptTokens,
		CompletionTokens:   document.CompletionTokens,
		TotalTokens:        document.TotalTokens,
		ReasoningTokens:    document.ReasoningTokens,
		PromptCachedTokens: document.PromptCachedTokens,
		CreatedAt:          document.CreatedAt,
	}
}

func AssetDocumentFromRecord(record repository.AssetRecord) po.AssetDocument {
	return po.AssetDocument{
		ID:          record.ID,
		SessionID:   record.SessionID,
		RequestID:   record.RequestID,
		ToolCallID:  record.ToolCallID,
		ToolName:    record.ToolName,
		LocalPath:   record.LocalPath,
		FileName:    record.FileName,
		ContentType: record.ContentType,
		Size:        record.Size,
		ShortURL:    record.ShortURL,
		ShortCode:   record.ShortCode,
		ExpiresAt:   record.ExpiresAt,
		Deleted:     record.Deleted,
		DeletedAt:   record.DeletedAt,
		CreatedAt:   record.CreatedAt,
	}
}

func AssetRecordFromDocument(document po.AssetDocument) repository.AssetRecord {
	return repository.AssetRecord{
		ID:          document.ID,
		SessionID:   document.SessionID,
		RequestID:   document.RequestID,
		ToolCallID:  document.ToolCallID,
		ToolName:    document.ToolName,
		LocalPath:   document.LocalPath,
		FileName:    document.FileName,
		ContentType: document.ContentType,
		Size:        document.Size,
		ShortURL:    document.ShortURL,
		ShortCode:   document.ShortCode,
		ExpiresAt:   document.ExpiresAt,
		Deleted:     document.Deleted,
		DeletedAt:   document.DeletedAt,
		CreatedAt:   document.CreatedAt,
	}
}

func ModelConfigDocumentFromDomain(config domainmodel.Config) po.ModelConfigDocument {
	return po.ModelConfigDocument{
		ID:        config.ID,
		Name:      config.Name,
		Provider:  config.Provider,
		BaseURL:   config.BaseURL,
		APIKey:    config.APIKey,
		ModelName: config.ModelName,
		Enabled:   config.Enabled,
		IsDefault: config.IsDefault,
		CreatedAt: config.CreatedAt,
		UpdatedAt: config.UpdatedAt,
	}
}

func ModelConfigDomainFromDocument(document po.ModelConfigDocument) domainmodel.Config {
	return domainmodel.Config{
		ID:        document.ID,
		Name:      document.Name,
		Provider:  document.Provider,
		BaseURL:   document.BaseURL,
		APIKey:    document.APIKey,
		ModelName: document.ModelName,
		Enabled:   document.Enabled,
		IsDefault: document.IsDefault,
		CreatedAt: document.CreatedAt,
		UpdatedAt: document.UpdatedAt,
	}
}

func TokenUsageDocumentFromRecord(record *repository.TokenUsageRecord) *po.TokenUsageDocument {
	if record == nil {
		return nil
	}
	return &po.TokenUsageDocument{
		PromptTokens:       record.PromptTokens,
		CompletionTokens:   record.CompletionTokens,
		TotalTokens:        record.TotalTokens,
		ReasoningTokens:    record.ReasoningTokens,
		PromptCachedTokens: record.PromptCachedTokens,
		Available:          record.Available,
	}
}

func TokenUsageRecordFromDocument(document *po.TokenUsageDocument) *repository.TokenUsageRecord {
	if document == nil {
		return nil
	}
	return &repository.TokenUsageRecord{
		PromptTokens:       document.PromptTokens,
		CompletionTokens:   document.CompletionTokens,
		TotalTokens:        document.TotalTokens,
		ReasoningTokens:    document.ReasoningTokens,
		PromptCachedTokens: document.PromptCachedTokens,
		Available:          document.Available,
	}
}

func PlanDocumentFromDomain(value *agentplan.Plan) *po.PlanDocument {
	if value == nil {
		return nil
	}
	steps := make([]po.PlanStepDocument, 0, len(value.Steps))
	for _, step := range value.Steps {
		steps = append(steps, po.PlanStepDocument{
			ID:          step.ID,
			Order:       step.Order,
			Title:       step.Title,
			Description: step.Description,
			Status:      step.Status,
		})
	}
	return &po.PlanDocument{
		ID:         value.ID,
		SessionID:  value.SessionID,
		Goal:       value.Goal,
		Status:     value.Status,
		RawContent: value.RawContent,
		Steps:      steps,
		CreatedAt:  value.CreatedAt,
		UpdatedAt:  value.UpdatedAt,
	}
}

func PlanDomainFromDocument(document *po.PlanDocument) *agentplan.Plan {
	if document == nil {
		return nil
	}
	steps := make([]agentplan.Step, 0, len(document.Steps))
	for _, step := range document.Steps {
		steps = append(steps, agentplan.Step{
			ID:          step.ID,
			Order:       step.Order,
			Title:       step.Title,
			Description: step.Description,
			Status:      step.Status,
		})
	}
	return &agentplan.Plan{
		ID:         document.ID,
		SessionID:  document.SessionID,
		Goal:       document.Goal,
		Status:     document.Status,
		RawContent: document.RawContent,
		Steps:      steps,
		CreatedAt:  document.CreatedAt,
		UpdatedAt:  document.UpdatedAt,
	}
}
