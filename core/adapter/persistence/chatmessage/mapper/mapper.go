package mapper

import (
	"time"

	uuidadapter "myai/core/adapter/id/uuid"
	chatmessageport "myai/core/adapter/persistence/chatmessage/port"
	generationcommand "myai/core/application/chat/generation/command"
	"myai/core/contextmgr"
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
	return repository.MessageRecord{
		ID:        m.newID(),
		SessionID: command.SessionID,
		Role:      repository.RoleUser,
		Content:   command.Input,
		CreatedAt: createdAt,
	}
}

func (m Mapper) UserTurn(command generationcommand.PersistUserMessage, createdAt time.Time) []repository.MessageRecord {
	records := make([]repository.MessageRecord, 0, 3)
	if ragMessage := domainmessage.RAGContext(command.RAGContext); ragMessage.IsSynthetic() {
		records = append(records, repository.MessageRecord{
			ID: m.newID(), SessionID: command.SessionID, Role: repository.RoleSystem,
			Content: ragMessage.Text(), SyntheticReason: string(ragMessage.SyntheticReason), CreatedAt: createdAt,
		})
	}
	if runtimeMessage := domainmessage.RuntimeInstruction(command.RuntimeInstruction); runtimeMessage.IsSynthetic() {
		records = append(records, repository.MessageRecord{
			ID:              m.newID(),
			SessionID:       command.SessionID,
			Role:            repository.RoleSystem,
			Content:         runtimeMessage.Text(),
			SyntheticReason: string(runtimeMessage.SyntheticReason),
			CreatedAt:       createdAt,
		})
	}
	userCreatedAt := createdAt.Add(time.Duration(len(records)) * time.Nanosecond)
	records = append(records, m.UserMessage(command, userCreatedAt))
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
		CreatedAt:          createdAt,
	}
}

func (m Mapper) Session(current *session.Session, title string) repository.SessionRecord {
	if current == nil {
		return repository.SessionRecord{}
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

	records := make([]repository.MessageRecord, 0, len(current.Messages))
	for _, message := range current.Messages {
		records = append(records, m.messageRecords(current.ID, message)...)
	}
	createdAt := m.now().Add(-time.Duration(len(records)) * time.Nanosecond)
	for index := range records {
		records[index].CreatedAt = createdAt.Add(time.Duration(index) * time.Nanosecond)
	}
	return records
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
