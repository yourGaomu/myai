package mapper

import (
	"myai/core/adapter/persistence/mongo/po"
	compaction "myai/core/domain/compaction"
	generation "myai/core/domain/generation"
	domainmodel "myai/core/domain/model"
	agentplan "myai/core/plan"
	repository "myai/core/port/repository"
	"myai/core/session"
)

func SessionDocumentFromRecord(record repository.SessionRecord) po.SessionDocument {
	// mapper 是 BSON PO 与仓库 Record 的唯一转换边界，内层对象不携带 bson 标签。
	return po.SessionDocument{
		ID:                   record.ID,
		Kind:                 record.Kind,
		ParentSessionID:      record.ParentSessionID,
		ParentTaskID:         record.ParentTaskID,
		AgentDefinitionID:    record.AgentDefinitionID,
		AgentDefinitionVer:   record.AgentDefinitionVer,
		SystemInstruction:    record.SystemInstruction,
		AllowedTools:         append([]string(nil), record.AllowedTools...),
		EnforceToolAllowlist: record.EnforceToolAllowlist,
		WorkspaceRoot:        record.WorkspaceRoot,
		WorkspaceSandboxID:   record.WorkspaceSandboxID,
		MaxToolRounds:        record.MaxToolRounds,
		Model:                record.Model,
		AgentMode:            record.AgentMode,
		PermissionMode:       record.PermissionMode,
		ContextWindowK:       record.ContextWindowK,
		Summary:              record.Summary,
		CompactedMessages:    record.CompactedMessages,
		CompactionSourceHash: record.CompactionSourceHash,
		CompactionCheckpoint: CompactionCheckpointDocumentFromDomain(record.CompactionCheckpoint),
		CompactedAt:          record.CompactedAt,
		Title:                record.Title,
		Usage:                TokenUsageDocumentFromRecord(record.Usage),
		LastUsage:            TokenUsageDocumentFromRecord(record.LastUsage),
		CurrentPlan:          PlanDocumentFromDomain(record.CurrentPlan),
		RAGSettings:          RAGSettingsDocumentFromDomain(record.RAGSettings),
		GenerationSettings:   GenerationSettingsDocumentFromDomain(record.GenerationSettings),
		StyleInstruction:     record.StyleInstruction,
		Deleted:              record.Deleted,
		DeletedAt:            record.DeletedAt,
		CreatedAt:            record.CreatedAt,
		UpdatedAt:            record.UpdatedAt,
	}
}

func SessionRecordFromDocument(document po.SessionDocument) repository.SessionRecord {
	return repository.SessionRecord{
		ID:                   document.ID,
		Kind:                 document.Kind,
		ParentSessionID:      document.ParentSessionID,
		ParentTaskID:         document.ParentTaskID,
		AgentDefinitionID:    document.AgentDefinitionID,
		AgentDefinitionVer:   document.AgentDefinitionVer,
		SystemInstruction:    document.SystemInstruction,
		AllowedTools:         append([]string(nil), document.AllowedTools...),
		EnforceToolAllowlist: document.EnforceToolAllowlist,
		WorkspaceRoot:        document.WorkspaceRoot,
		WorkspaceSandboxID:   document.WorkspaceSandboxID,
		MaxToolRounds:        document.MaxToolRounds,
		Model:                document.Model,
		AgentMode:            document.AgentMode,
		PermissionMode:       document.PermissionMode,
		ContextWindowK:       document.ContextWindowK,
		Summary:              document.Summary,
		CompactedMessages:    document.CompactedMessages,
		CompactionSourceHash: document.CompactionSourceHash,
		CompactionCheckpoint: CompactionCheckpointDomainFromDocument(document.CompactionCheckpoint),
		CompactedAt:          document.CompactedAt,
		Title:                document.Title,
		Usage:                TokenUsageRecordFromDocument(document.Usage),
		LastUsage:            TokenUsageRecordFromDocument(document.LastUsage),
		CurrentPlan:          PlanDomainFromDocument(document.CurrentPlan),
		RAGSettings:          RAGSettingsDomainFromDocument(document.RAGSettings),
		GenerationSettings:   GenerationSettingsDomainFromDocument(document.GenerationSettings),
		StyleInstruction:     document.StyleInstruction,
		Deleted:              document.Deleted,
		DeletedAt:            document.DeletedAt,
		CreatedAt:            document.CreatedAt,
		UpdatedAt:            document.UpdatedAt,
	}
}

func CompactionCheckpointDocumentFromDomain(checkpoint *compaction.Checkpoint) *po.CompactionCheckpointDocument {
	if checkpoint == nil {
		return nil
	}
	return &po.CompactionCheckpointDocument{
		Version: checkpoint.Version, SourceStartMessage: checkpoint.SourceStartMessage,
		SourceEndMessage: checkpoint.SourceEndMessage, SourceHistoryHash: checkpoint.SourceHistoryHash,
		Summary: checkpoint.Summary, SummaryData: CompactionSummaryDocumentFromDomain(checkpoint.SummaryData),
		CreatedAt: checkpoint.CreatedAt,
	}
}

func CompactionSummaryDocumentFromDomain(summary compaction.Summary) *po.CompactionSummaryDocument {
	return &po.CompactionSummaryDocument{
		CurrentGoal: summary.CurrentGoal, Preferences: append([]string(nil), summary.Preferences...),
		Constraints: append([]string(nil), summary.Constraints...), Decisions: append([]string(nil), summary.Decisions...),
		CompletedWork: append([]string(nil), summary.CompletedWork...), ModifiedFiles: append([]string(nil), summary.ModifiedFiles...),
		ToolVerification: append([]string(nil), summary.ToolVerification...), Problems: append([]string(nil), summary.Problems...),
		OpenTasks: append([]string(nil), summary.OpenTasks...), NextSteps: append([]string(nil), summary.NextSteps...),
		References: append([]string(nil), summary.References...),
	}
}

func CompactionCheckpointDomainFromDocument(document *po.CompactionCheckpointDocument) *compaction.Checkpoint {
	if document == nil {
		return nil
	}
	checkpoint := &compaction.Checkpoint{
		Version: document.Version, SourceStartMessage: document.SourceStartMessage,
		SourceEndMessage: document.SourceEndMessage, SourceHistoryHash: document.SourceHistoryHash,
		Summary: document.Summary, CreatedAt: document.CreatedAt,
	}
	if parsed, err := compaction.DecodeJSON(checkpoint.Summary); err == nil {
		// Canonical JSON is authoritative and also restores empty arrays that
		// Mongo omitempty may have omitted from the nested document.
		checkpoint.SummaryData = parsed
	} else if document.SummaryData != nil {
		checkpoint.SummaryData = compaction.Summary{
			CurrentGoal: document.SummaryData.CurrentGoal, Preferences: append([]string(nil), document.SummaryData.Preferences...),
			Constraints: append([]string(nil), document.SummaryData.Constraints...), Decisions: append([]string(nil), document.SummaryData.Decisions...),
			CompletedWork: append([]string(nil), document.SummaryData.CompletedWork...), ModifiedFiles: append([]string(nil), document.SummaryData.ModifiedFiles...),
			ToolVerification: append([]string(nil), document.SummaryData.ToolVerification...), Problems: append([]string(nil), document.SummaryData.Problems...),
			OpenTasks: append([]string(nil), document.SummaryData.OpenTasks...), NextSteps: append([]string(nil), document.SummaryData.NextSteps...),
			References: append([]string(nil), document.SummaryData.References...),
		}
	} else {
		checkpoint.SummaryData = compaction.ParseSummary(checkpoint.Summary)
	}
	return checkpoint
}

func MessageDocumentFromRecord(record repository.MessageRecord) po.MessageDocument {
	return po.MessageDocument{
		ID:                  record.ID,
		SessionID:           record.SessionID,
		Role:                record.Role,
		Content:             record.Content,
		Reasoning:           record.Reasoning,
		ToolCallID:          record.ToolCallID,
		ToolName:            record.ToolName,
		ToolArguments:       record.ToolArguments,
		ToolError:           record.ToolError,
		ToolStatus:          record.ToolStatus,
		ToolErrorCode:       record.ToolErrorCode,
		ToolTruncated:       record.ToolTruncated,
		ToolPromptContent:   record.ToolPromptContent,
		ToolPromptError:     record.ToolPromptError,
		ToolPromptTruncated: record.ToolPromptTruncated,
		SyntheticReason:     record.SyntheticReason,
		PromptTokens:        record.PromptTokens,
		CompletionTokens:    record.CompletionTokens,
		TotalTokens:         record.TotalTokens,
		ReasoningTokens:     record.ReasoningTokens,
		PromptCachedTokens:  record.PromptCachedTokens,
		Sequence:            record.Sequence,
		CreatedAt:           record.CreatedAt,
	}
}

func MessageRecordFromDocument(document po.MessageDocument) repository.MessageRecord {
	return repository.MessageRecord{
		ID:                  document.ID,
		SessionID:           document.SessionID,
		Role:                document.Role,
		Content:             document.Content,
		Reasoning:           document.Reasoning,
		ToolCallID:          document.ToolCallID,
		ToolName:            document.ToolName,
		ToolArguments:       document.ToolArguments,
		ToolError:           document.ToolError,
		ToolStatus:          document.ToolStatus,
		ToolErrorCode:       document.ToolErrorCode,
		ToolTruncated:       document.ToolTruncated,
		ToolPromptContent:   document.ToolPromptContent,
		ToolPromptError:     document.ToolPromptError,
		ToolPromptTruncated: document.ToolPromptTruncated,
		SyntheticReason:     document.SyntheticReason,
		PromptTokens:        document.PromptTokens,
		CompletionTokens:    document.CompletionTokens,
		TotalTokens:         document.TotalTokens,
		ReasoningTokens:     document.ReasoningTokens,
		PromptCachedTokens:  document.PromptCachedTokens,
		Sequence:            document.Sequence,
		CreatedAt:           document.CreatedAt,
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
		ID:                        config.ID,
		Name:                      config.Name,
		Provider:                  config.Provider,
		Protocol:                  string(config.Protocol),
		AuthType:                  string(config.AuthType),
		BaseURL:                   config.BaseURL,
		APIKey:                    config.APIKey,
		ModelName:                 config.ModelName,
		Enabled:                   config.Enabled,
		IsDefault:                 config.IsDefault,
		DefaultGenerationSettings: GenerationSettingsDocumentFromDomain(config.DefaultGenerationSettings),
		CreatedAt:                 config.CreatedAt,
		UpdatedAt:                 config.UpdatedAt,
	}
}

func ModelConfigDomainFromDocument(document po.ModelConfigDocument) domainmodel.Config {
	return domainmodel.Config{
		ID:                        document.ID,
		Name:                      document.Name,
		Provider:                  document.Provider,
		Protocol:                  domainmodel.Protocol(document.Protocol),
		AuthType:                  domainmodel.AuthType(document.AuthType),
		BaseURL:                   document.BaseURL,
		APIKey:                    document.APIKey,
		ModelName:                 document.ModelName,
		Enabled:                   document.Enabled,
		IsDefault:                 document.IsDefault,
		DefaultGenerationSettings: GenerationSettingsDomainFromDocument(document.DefaultGenerationSettings),
		CreatedAt:                 document.CreatedAt,
		UpdatedAt:                 document.UpdatedAt,
	}
}

func GenerationSettingsDocumentFromDomain(settings generation.Settings) *po.GenerationSettingsDocument {
	if settings.Temperature == nil && settings.TopP == nil && settings.MaxOutputTokens == nil {
		return nil
	}
	return &po.GenerationSettingsDocument{
		Temperature:     cloneFloat64(settings.Temperature),
		TopP:            cloneFloat64(settings.TopP),
		MaxOutputTokens: cloneInt(settings.MaxOutputTokens),
	}
}

func RAGSettingsDocumentFromDomain(settings session.RAGSettings) *po.RAGSettingsDocument {
	settings = session.NormalizeRAGSettings(settings)
	if settings.Mode == session.RetrievalModeAuto && settings.TopK == session.DefaultRAGTopK && len(settings.KnowledgeBaseIDs) == 0 && len(settings.CategoryIDs) == 0 {
		return nil
	}
	return &po.RAGSettingsDocument{
		Mode:             string(settings.Mode),
		KnowledgeBaseIDs: append([]string(nil), settings.KnowledgeBaseIDs...),
		CategoryIDs:      append([]string(nil), settings.CategoryIDs...),
		TopK:             settings.TopK,
	}
}

func RAGSettingsDomainFromDocument(document *po.RAGSettingsDocument) session.RAGSettings {
	if document == nil {
		return session.DefaultRAGSettings()
	}
	return session.CloneRAGSettings(session.RAGSettings{
		Mode:             session.RetrievalMode(document.Mode),
		KnowledgeBaseIDs: document.KnowledgeBaseIDs,
		CategoryIDs:      document.CategoryIDs,
		TopK:             document.TopK,
	})
}

func GenerationSettingsDomainFromDocument(document *po.GenerationSettingsDocument) generation.Settings {
	if document == nil {
		return generation.Settings{}
	}
	return generation.Settings{
		Temperature:     cloneFloat64(document.Temperature),
		TopP:            cloneFloat64(document.TopP),
		MaxOutputTokens: cloneInt(document.MaxOutputTokens),
	}
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
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
			ID:           step.ID,
			Order:        step.Order,
			Title:        step.Title,
			Description:  step.Description,
			Dependencies: append([]string(nil), step.Dependencies...),
			Status:       step.Status,
			RetryCount:   step.RetryCount,
			MaxRetries:   step.MaxRetries,
			LastError:    step.LastError,
			AgentTaskID:  step.AgentTaskID,
			StartedAt:    step.StartedAt,
			CompletedAt:  step.CompletedAt,
		})
	}
	return &po.PlanDocument{
		ID:         value.ID,
		SessionID:  value.SessionID,
		Goal:       value.Goal,
		Status:     value.Status,
		Revision:   value.Revision,
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
			ID:           step.ID,
			Order:        step.Order,
			Title:        step.Title,
			Description:  step.Description,
			Dependencies: append([]string(nil), step.Dependencies...),
			Status:       step.Status,
			RetryCount:   step.RetryCount,
			MaxRetries:   step.MaxRetries,
			LastError:    step.LastError,
			AgentTaskID:  step.AgentTaskID,
			StartedAt:    step.StartedAt,
			CompletedAt:  step.CompletedAt,
		})
	}
	return &agentplan.Plan{
		ID:         document.ID,
		SessionID:  document.SessionID,
		Goal:       document.Goal,
		Status:     document.Status,
		Revision:   document.Revision,
		RawContent: document.RawContent,
		Steps:      steps,
		CreatedAt:  document.CreatedAt,
		UpdatedAt:  document.UpdatedAt,
	}
}
