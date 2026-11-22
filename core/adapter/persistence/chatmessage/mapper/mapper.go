package mapper

import (
	"slices"
	"time"

	uuidadapter "myai/core/adapter/id/uuid"
	chatmessageport "myai/core/adapter/persistence/chatmessage/port"
	generationcommand "myai/core/application/chat/generation/command"
	compaction "myai/core/domain/compaction"
	generation "myai/core/domain/generation"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	agentplan "myai/core/plan"
	modelport "myai/core/port/model"
	repository "myai/core/port/repository"
	"myai/core/session"
)

type Mapper struct {
	IDs chatmessageport.IDGenerator
	Now func() time.Time
}

func (m Mapper) UserMessage(command generationcommand.PersistUserMessage, createdAt time.Time) repository.MessageRecord {
	record := repository.MessageRecord{
		ID:              m.newID(),
		SessionID:       command.SessionID,
		Role:            repository.RoleUser,
		Content:         command.Input,
		SyntheticReason: string(command.SyntheticReason),
		Sequence:        messageSequence(createdAt),
		CreatedAt:       createdAt,
	}
	if message, ok := latestSnapshotMessage(command.SessionSnapshot, domainmessage.RoleUser, command.Input, command.SyntheticReason); ok {
		record.ID = message.HistoryRecordID(0, 1)
		copyPrivateMetadata(&record, message)
	}
	return record
}

func (m Mapper) UserTurn(command generationcommand.PersistUserMessage, createdAt time.Time) []repository.MessageRecord {
	if len(command.AppendedMessages) > 0 {
		return m.recordsForMessages(command.SessionID, command.AppendedMessages, createdAt)
	}
	records := make([]repository.MessageRecord, 0, 3)
	if ragMessage := domainmessage.RAGContext(command.RAGContext); ragMessage.IsSynthetic() {
		id := m.newID()
		if message, ok := latestSnapshotMessage(command.SessionSnapshot, domainmessage.RoleUser, ragMessage.Text(), ragMessage.SyntheticReason); ok {
			id = message.HistoryRecordID(0, 1)
		}
		records = append(records, repository.MessageRecord{
			ID: id, SessionID: command.SessionID, Role: repository.RoleUser,
			Content: ragMessage.Text(), SyntheticReason: string(ragMessage.SyntheticReason), CreatedAt: createdAt,
		})
	}
	if runtimeMessage := domainmessage.RuntimeInstruction(command.RuntimeInstruction); runtimeMessage.IsSynthetic() {
		record := repository.MessageRecord{
			ID:              m.newID(),
			SessionID:       command.SessionID,
			Role:            string(runtimeMessage.Role),
			Content:         runtimeMessage.Text(),
			SyntheticReason: string(runtimeMessage.SyntheticReason),
			CreatedAt:       createdAt,
		}
		if message, ok := latestSnapshotMessage(command.SessionSnapshot, runtimeMessage.Role, runtimeMessage.Text(), runtimeMessage.SyntheticReason); ok {
			record.ID = message.HistoryRecordID(0, 1)
			copyPrivateMetadata(&record, message)
		}
		records = append(records, record)
	}
	records = append(records, m.UserMessage(command, createdAt))
	for index := range records {
		messageCreatedAt := createdAt.Add(time.Duration(index) * time.Nanosecond)
		records[index].CreatedAt = messageCreatedAt
		records[index].Sequence = messageSequence(messageCreatedAt)
	}
	return records
}

func (m Mapper) AssistantMessage(sessionID string, result modelport.ChatResult, createdAt time.Time) repository.MessageRecord {
	return repository.MessageRecord{
		ID:                    m.newID(),
		SessionID:             sessionID,
		Role:                  repository.RoleAssistant,
		Content:               result.Content,
		Reasoning:             result.Reasoning,
		PromptTokens:          result.Usage.PromptTokens,
		CompletionTokens:      result.Usage.CompletionTokens,
		TotalTokens:           result.Usage.TotalTokens,
		ReasoningTokens:       result.Usage.ReasoningTokens,
		PromptCachedTokens:    result.Usage.PromptCachedTokens,
		PromptCacheReported:   result.Usage.PromptCacheReported,
		PromptCacheIncomplete: result.Usage.PromptCacheIncomplete,
		Sequence:              messageSequence(createdAt),
		CreatedAt:             createdAt,
	}
}

// AssistantTurn maps the exact message accepted by the transcript boundary.
// Callers without a committed message retain the standalone legacy write.
func (m Mapper) AssistantTurn(sessionID string, result modelport.ChatResult, createdAt time.Time) []repository.MessageRecord {
	if result.Message == nil || result.Message.Role != domainmessage.RoleAssistant || result.Message.ID == "" {
		return []repository.MessageRecord{m.AssistantMessage(sessionID, result, createdAt)}
	}
	records := m.recordsForMessages(sessionID, []domainmessage.Message{*result.Message}, createdAt)
	if reply := &records[0]; reply.Role == repository.RoleAssistant {
		reply.PromptTokens = result.Usage.PromptTokens
		reply.CompletionTokens = result.Usage.CompletionTokens
		reply.TotalTokens = result.Usage.TotalTokens
		reply.ReasoningTokens = result.Usage.ReasoningTokens
		reply.PromptCachedTokens = result.Usage.PromptCachedTokens
		reply.PromptCacheReported = result.Usage.PromptCacheReported
		reply.PromptCacheIncomplete = result.Usage.PromptCacheIncomplete
	}
	return records
}

func (m Mapper) Session(current *session.Session, title string) repository.SessionRecord {
	if current == nil {
		return repository.SessionRecord{}
	}
	var checkpoint *compaction.Checkpoint
	if current.CompactionCheckpoint != nil {
		checkpoint = current.CompactionCheckpoint.Clone()
	}
	return repository.SessionRecord{
		ID:                   current.ID,
		Kind:                 string(session.NormalizeKind(current.Kind)),
		ParentSessionID:      current.ParentSessionID,
		ParentTaskID:         current.ParentTaskID,
		AgentDefinitionID:    current.AgentDefinitionID,
		AgentDefinitionVer:   current.AgentDefinitionVer,
		SystemInstruction:    current.SystemInstruction,
		AllowedTools:         append([]string(nil), current.AllowedTools...),
		EnforceToolAllowlist: current.EnforceToolAllowlist,
		PermissionCeiling:    session.ClonePermissionCeiling(current.PermissionCeiling),
		WorkspaceRoot:        current.WorkspaceRoot,
		WorkspaceSandboxID:   current.WorkspaceSandboxID,
		MaxToolRounds:        current.MaxToolRounds,
		Model:                current.Model,
		AgentMode:            string(session.NormalizeAgentMode(current.AgentMode)),
		PermissionMode:       string(session.NormalizePermissionMode(current.PermissionMode)),
		ContextWindowK:       current.ContextWindowK,
		Summary:              current.Summary,
		CompactedMessages:    current.CompactedMessages,
		CompactionSourceHash: current.CompactionSourceHash,
		CompactionCheckpoint: checkpoint,
		Title:                title,
		Usage:                tokenUsage(current.Usage),
		LastUsage:            tokenUsage(current.LastUsage),
		CurrentPlan:          agentplan.Clone(current.CurrentPlan),
		RAGSettings:          session.CloneRAGSettings(current.RAGSettings),
		GenerationSettings:   generation.Clone(current.GenerationSettings),
		StyleInstruction:     current.StyleInstruction,
	}
}

func (m Mapper) MemoryMessages(current *session.Session) []repository.MessageRecord {
	if current == nil {
		return nil
	}

	count := 0
	for _, message := range current.Messages {
		count += messageRecordCount(message)
	}
	createdAt := m.now().Add(-time.Duration(count) * time.Nanosecond)
	return m.recordsForMessages(current.ID, current.Messages, createdAt)
}

func messageRecordCount(message domainmessage.Message) int {
	switch message.Role {
	case domainmessage.RoleSystem:
		if !message.IsSynthetic() {
			return 0
		}
		return 1
	case domainmessage.RoleUser:
		return 1
	case domainmessage.RoleAssistant:
		count := 0
		if message.Text() != "" || message.Reasoning != "" {
			count++
		}
		for _, part := range message.Parts {
			if part.Type == domainmessage.PartToolCall && part.ToolCall != nil {
				count++
			}
		}
		if count == 0 {
			return 1
		}
		return count
	case domainmessage.RoleTool:
		count := 0
		for _, part := range message.Parts {
			if part.Type == domainmessage.PartToolResult && part.ToolResult != nil {
				count++
			}
		}
		if count == 0 {
			return 1
		}
		return count
	default:
		return 0
	}
}

func (m Mapper) recordsForMessages(sessionID string, messages []domainmessage.Message, fallbackCreatedAt time.Time) []repository.MessageRecord {
	records := make([]repository.MessageRecord, 0, len(messages))
	for _, message := range messages {
		mapped := m.messageRecords(sessionID, message)
		for partIndex := range mapped {
			mapped[partIndex].MessageID = message.ID
			if id := message.HistoryRecordID(partIndex, len(mapped)); id != "" {
				mapped[partIndex].ID = id
			}
			createdAt := fallbackCreatedAt.Add(time.Duration(len(records)) * time.Nanosecond)
			if !message.CreatedAt.IsZero() {
				createdAt = message.CreatedAt
			}
			mapped[partIndex].CreatedAt = createdAt
			mapped[partIndex].Sequence = message.Sequence
			// 1. assistant/tool 消息的每个记录也要保留来源字段，避免映射时丢失事件身份。
			mapped[partIndex].SourceID = message.SourceID
			mapped[partIndex].SourceKind = message.SourceKind
			mapped[partIndex].SourceTaskID = message.SourceTaskID
			// Private metadata belongs to the message, even when its first row is a tool call.
			if partIndex == 0 {
				copyPrivateMetadata(&mapped[partIndex], message)
			}
			if mapped[partIndex].Sequence <= 0 {
				mapped[partIndex].Sequence = messageSequence(createdAt)
			}
			records = append(records, mapped[partIndex])
		}
	}
	return records
}

func copyPrivateMetadata(record *repository.MessageRecord, message domainmessage.Message) {
	record.ImageReceipts = slices.Clone(message.ImageReceipts)
	record.RequestContext = message.RequestContext.Clone()
	record.SkillPromptTokens = message.SkillPromptTokens
}

func latestSnapshotMessage(snapshot *session.Session, role domainmessage.Role, content string, reason domainmessage.SyntheticReason) (domainmessage.Message, bool) {
	if snapshot == nil {
		return domainmessage.Message{}, false
	}
	for index := len(snapshot.Messages) - 1; index >= 0; index-- {
		message := snapshot.Messages[index]
		if message.Role == role && message.Text() == content && (reason == "" || message.SyntheticReason == reason) {
			return message, true
		}
	}
	return domainmessage.Message{}, false
}

func (m Mapper) messageRecords(sessionID string, message domainmessage.Message) []repository.MessageRecord {
	newRecord := func(role string) repository.MessageRecord {
		return repository.MessageRecord{ID: m.newID(), SessionID: sessionID, Role: role}
	}
	switch message.Role {
	case domainmessage.RoleSystem:
		if !message.IsSynthetic() {
			return nil
		}
		record := newRecord(repository.RoleSystem)
		record.Content = message.Text()
		record.SyntheticReason = string(message.SyntheticReason)
		// 2. system/user 消息沿用同一来源元数据，保证不同角色的持久化格式一致。
		record.SourceID = message.SourceID
		record.SourceKind = message.SourceKind
		record.SourceTaskID = message.SourceTaskID
		return []repository.MessageRecord{record}
	case domainmessage.RoleUser:
		record := newRecord(repository.RoleUser)
		record.Content = message.Text()
		record.SyntheticReason = string(message.SyntheticReason)
		record.SourceID = message.SourceID
		record.SourceKind = message.SourceKind
		record.SourceTaskID = message.SourceTaskID
		return []repository.MessageRecord{record}
	case domainmessage.RoleAssistant:
		records := make([]repository.MessageRecord, 0, len(message.Parts)+1)
		usage := message.AssistantUsage
		if text := message.Text(); text != "" || message.Reasoning != "" || usage != (domainmessage.AssistantUsage{}) {
			record := newRecord(repository.RoleAssistant)
			record.Content = text
			record.Reasoning = message.Reasoning
			record.PromptTokens = usage.PromptTokens
			record.CompletionTokens = usage.CompletionTokens
			record.TotalTokens = usage.TotalTokens
			record.ReasoningTokens = usage.ReasoningTokens
			record.PromptCachedTokens = usage.PromptCachedTokens
			record.PromptCacheReported = usage.PromptCacheReported
			record.PromptCacheIncomplete = usage.PromptCacheIncomplete
			records = append(records, record)
		}
		for _, part := range message.Parts {
			if part.Type != domainmessage.PartToolCall || part.ToolCall == nil {
				continue
			}
			record := newRecord(repository.RoleToolCall)
			record.ToolCallID = part.ToolCall.ID
			record.ToolName = part.ToolCall.Name
			record.ToolArguments = part.ToolCall.Arguments
			records = append(records, record)
		}
		if len(records) == 0 {
			records = append(records, newRecord(repository.RoleAssistant))
		}
		return records
	case domainmessage.RoleTool:
		records := make([]repository.MessageRecord, 0, len(message.Parts))
		for _, part := range message.Parts {
			if part.Type != domainmessage.PartToolResult || part.ToolResult == nil {
				continue
			}
			result := part.ToolResult
			record := newRecord(repository.RoleTool)
			status := result.Status
			if status == "" {
				status = domaintool.ResultStatusSuccess
			}
			record.Content = result.Content
			record.ToolCallID = result.ToolCallID
			record.ToolName = result.Name
			record.ToolStatus = string(status)
			record.ToolError = result.ErrorMessage
			record.ToolErrorCode = result.ErrorCode
			record.ToolTruncated = result.Truncated
			if result.PromptTruncated {
				record.Content = result.FullContent
				record.ToolError = result.FullErrorMessage
				record.ToolPromptContent = result.Content
				record.ToolPromptError = result.ErrorMessage
				record.ToolPromptTruncated = true
			}
			records = append(records, record)
		}
		if len(records) == 0 {
			record := newRecord(repository.RoleTool)
			record.Content = message.Text()
			records = append(records, record)
		}
		return records
	default:
		return nil
	}
}

func (m Mapper) newID() string {
	if m.IDs != nil {
		return m.IDs.NewID()
	}
	return (uuidadapter.Generator{}).NewID()
}

func (m Mapper) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func messageSequence(createdAt time.Time) int64 {
	if createdAt.IsZero() {
		return 0
	}
	return createdAt.UnixNano()
}

func tokenUsage(usage modelport.TokenUsage) *repository.TokenUsageRecord {
	if usage == (modelport.TokenUsage{}) {
		return nil
	}
	return &repository.TokenUsageRecord{
		PromptTokens:          usage.PromptTokens,
		CompletionTokens:      usage.CompletionTokens,
		TotalTokens:           usage.TotalTokens,
		ReasoningTokens:       usage.ReasoningTokens,
		PromptCachedTokens:    usage.PromptCachedTokens,
		PromptCacheReported:   usage.PromptCacheReported,
		PromptCacheIncomplete: usage.PromptCacheIncomplete,
		Available:             usage.Available,
	}
}
