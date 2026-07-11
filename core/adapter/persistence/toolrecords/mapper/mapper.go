package mapper

import (
	uuidadapter "myai/core/adapter/id/uuid"
	toolrecordsport "myai/core/adapter/persistence/toolrecords/port"
	domaintool "myai/core/domain/tool"
	repository "myai/core/port/repository"
)

type Mapper struct {
	IDs toolrecordsport.IDGenerator
}

func (m Mapper) MessageRecords(entries []domaintool.ExecutionEntry) []repository.MessageRecord {
	records := make([]repository.MessageRecord, 0, len(entries))
	for _, entry := range entries {
		record, ok := m.messageRecord(entry)
		if ok {
			records = append(records, record)
		}
	}
	return records
}

func (m Mapper) AssetRecords(assets []domaintool.SharedAsset) []repository.AssetRecord {
	records := make([]repository.AssetRecord, 0, len(assets))
	for _, asset := range assets {
		records = append(records, repository.AssetRecord{
			ID:          m.newID(),
			SessionID:   asset.SessionID,
			RequestID:   asset.RequestID,
			ToolCallID:  asset.ToolCallID,
			ToolName:    asset.ToolName,
			LocalPath:   asset.LocalPath,
			FileName:    asset.FileName,
			ContentType: asset.ContentType,
			Size:        asset.Size,
			ShortURL:    asset.ShortURL,
			ShortCode:   asset.ShortCode,
			ExpiresAt:   asset.ExpiresAt,
			CreatedAt:   asset.CreatedAt,
		})
	}
	return records
}

func (m Mapper) messageRecord(entry domaintool.ExecutionEntry) (repository.MessageRecord, bool) {
	role := ""
	switch entry.Kind {
	case domaintool.ExecutionEntryToolCall:
		role = repository.RoleToolCall
	case domaintool.ExecutionEntryToolResult:
		role = repository.RoleTool
	default:
		return repository.MessageRecord{}, false
	}

	return repository.MessageRecord{
		ID:            m.newID(),
		SessionID:     entry.SessionID,
		Role:          role,
		Content:       entry.Content,
		ToolCallID:    entry.ToolCallID,
		ToolName:      entry.ToolName,
		ToolArguments: entry.Arguments,
		ToolError:     entry.Error,
		CreatedAt:     entry.CreatedAt,
	}, true
}

func (m Mapper) newID() string {
	if m.IDs != nil {
		return m.IDs.NewID()
	}
	return (uuidadapter.Generator{}).NewID()
}
