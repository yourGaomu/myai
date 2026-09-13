package mapper

import (
	"fmt"
	"time"

	uuidadapter "myai/core/adapter/id/uuid"
	chatmessageport "myai/core/adapter/persistence/chatmessage/port"
	generationcommand "myai/core/application/chat/generation/command"
	"myai/core/contextmgr"
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
	id := m.newID()
	if message, ok := latestSnapshotMessage(command.SessionSnapshot, domainmessage.RoleUser, command.Input, command.SyntheticReason); ok {
		id = messageRecordID(message, 0, 1)
	}
	return repository.MessageRecord{
		ID:              id,
		SessionID:       command.SessionID,
		Role:            repository.RoleUser,
		Content:         command.Input,
		SyntheticReason: string(command.SyntheticReason),
		Sequence:        messageSequence(createdAt),
		CreatedAt:       createdAt,
	}
}

func (m Mapper) UserTurn(command generationcommand.PersistUserMessage, createdAt time.Time) []repository.MessageRecord {
	if len(command.AppendedMessages) > 0 {
		return m.recordsForMessages(command.SessionID, command.AppendedMessages, createdAt)
	}
	records := make([]repository.MessageRecord, 0, 3)
	if ragMessage := domainmessage.RAGContext(command.RAGContext); ragMessage.IsSynthetic() {
		id := m.newID()
		if message, ok := latestSnapshotMessage(command.SessionSnapshot, domainmessage.RoleUser, ragMessage.Text(), ragMessage.SyntheticReason); ok {
			id = messageRecordID(message, 0, 1)
		}
		records = append(records, repository.MessageRecord{
			ID: id, SessionID: command.SessionID, Role: repository.RoleUser,
			Content: ragMessage.Text(), SyntheticReason: string(ragMessage.SyntheticReason), CreatedAt: createdAt,
		})
	}
	if runtimeMessage := domainmessage.RuntimeInstruction(command.RuntimeInstruction); runtimeMessage.IsSynthetic() {
		id := m.newID()
		if message, ok := latestSnapshotMessage(command.SessionSnapshot, domainmessage.RoleSystem, runtimeMessage.Text(), runtimeMessage.SyntheticReason); ok {
			id = messageRecordID(message, 0, 1)
		}
		records = append(records, repository.MessageRecord{
			ID:              id,
			SessionID:       command.SessionID,
			Role:            repository.RoleSystem,
			Content:         runtimeMessage.Text(),
			SyntheticReason: string(runtimeMessage.SyntheticReason),
			CreatedAt:       createdAt,
		})
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
		ID:                 m.newID(),
		SessionID:          sessionID,
		Role:               repository.RoleAssistant,
		Content:            result.Content,
		Reasoning:          result.Reasoning,
		PromptTokens:       result.Usage.PromptTokens,
		CompletionTokens:   result.Usage.CompletionTokens,
		TotalTokens:        result.Usage.TotalTokens,
		ReasoningTokens:    result.Usage.ReasoningTokens,
		PromptCachedTokens: result.Usage.PromptCachedTokens,
		Sequence:           messageSequence(createdAt),
		CreatedAt:          createdAt,
	}
}

func (m Mapper) AssistantMessageFromSession(current *session.Session, result modelport.ChatResult, createdAt time.Time) repository.MessageRecord {
	sessionID := ""
	if current != nil {
		sessionID = current.ID
	}
	record := m.AssistantMessage(sessionID, result, createdAt)
	if message, ok := latestSnapshotMessage(current, domainmessage.RoleAssistant, result.Content, ""); ok {
		record.ID = messageRecordID(message, 0, 1)
		if message.Sequence > 0 {
			record.Sequence = message.Sequence
		}
		if !message.CreatedAt.IsZero() {
			record.CreatedAt = message.CreatedAt
		}
	}
	return record
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
		WorkspaceRoot:        current.WorkspaceRoot,
		WorkspaceSandboxID:   current.WorkspaceSandboxID,
		MaxToolRounds:        current.MaxToolRounds,
		Model:                current.Model,
		AgentMode:            string(session.NormalizeAgentMode(current.AgentMode)),
		PermissionMode:       string(session.NormalizePermissionMode(current.PermissionMode)),
		ContextWindowK:       contextmgr.NormalizeWindowK(current.ContextWindowK),
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
		if message.Text() != "" {
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
			if id := messageRecordID(message, partIndex, len(mapped)); id != "" {
				mapped[partIndex].ID = id
			}
			createdAt := fallbackCreatedAt.Add(time.Duration(len(records)) * time.Nanosecond)
			if !message.CreatedAt.IsZero() {
				createdAt = message.CreatedAt
			}
			mapped[partIndex].CreatedAt = createdAt
			mapped[partIndex].Sequence = message.Sequence
			if mapped[partIndex].Sequence <= 0 {
				mapped[partIndex].Sequence = messageSequence(createdAt)
			}
			records = append(records, mapped[partIndex])
		}
	}
	return records
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

func messageRecordID(message domainmessage.Message, partIndex, partCount int) string {
	if partIndex < len(message.RecordIDs) && message.RecordIDs[partIndex] != "" {
		return message.RecordIDs[partIndex]
	}
	if message.ID == "" {
		return ""
	}
	if partCount <= 1 {
		return message.ID
	}
	return fmt.Sprintf("%s:%06d", message.ID, partIndex)
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
		return []repository.MessageRecord{record}
	case domainmessage.RoleUser:
		record := newRecord(repository.RoleUser)
		record.Content = message.Text()
		record.SyntheticReason = string(message.SyntheticReason)
		return []repository.MessageRecord{record}
	case domainmessage.RoleAssistant:
		records := make([]repository.MessageRecord, 0, len(message.Parts)+1)
		if text := message.Text(); text != "" {
			record := newRecord(repository.RoleAssistant)
			record.Content = text
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
	if !usage.Available && usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 && usage.ReasoningTokens == 0 && usage.PromptCachedTokens == 0 {
		return nil
	}
	return &repository.TokenUsageRecord{
		PromptTokens:       usage.PromptTokens,
		CompletionTokens:   usage.CompletionTokens,
		TotalTokens:        usage.TotalTokens,
		ReasoningTokens:    usage.ReasoningTokens,
		PromptCachedTokens: usage.PromptCachedTokens,
		Available:          usage.Available,
	}
}
