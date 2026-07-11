package mapper

import (
	"time"

	uuidadapter "myai/core/adapter/id/uuid"
	chatmessageport "myai/core/adapter/persistence/chatmessage/port"
	generationcommand "myai/core/application/chat/generation/command"
	"myai/core/contextmgr"
	domainmessage "myai/core/domain/message"
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
		ID:                current.ID,
		Model:             current.Model,
		AgentMode:         string(session.NormalizeAgentMode(current.AgentMode)),
		PermissionMode:    string(session.NormalizePermissionMode(current.PermissionMode)),
		ContextWindowK:    contextmgr.NormalizeWindowK(current.ContextWindowK),
		Summary:           current.Summary,
		CompactedMessages: current.CompactedMessages,
		Title:             title,
		Usage:             tokenUsage(current.Usage),
		LastUsage:         tokenUsage(current.LastUsage),
		CurrentPlan:       agentplan.Clone(current.CurrentPlan),
	}
}

func (m Mapper) MemoryMessages(current *session.Session) []repository.MessageRecord {
	if current == nil {
		return nil
	}

	records := make([]repository.MessageRecord, 0, len(current.Messages))
	createdAt := m.now().Add(-time.Duration(len(current.Messages)) * time.Nanosecond)
	for index, message := range current.Messages {
		record, ok := m.messageRecord(current.ID, message, createdAt.Add(time.Duration(index)*time.Nanosecond))
		if ok {
			records = append(records, record)
		}
	}
	return records
}

func (m Mapper) messageRecord(sessionID string, message domainmessage.Message, createdAt time.Time) (repository.MessageRecord, bool) {
	record := repository.MessageRecord{
		SessionID: sessionID,
		CreatedAt: createdAt,
	}
	switch message.Role {
	case domainmessage.RoleSystem:
		return repository.MessageRecord{}, false
	case domainmessage.RoleUser:
		record.Role = repository.RoleUser
		record.Content = message.Text()
	case domainmessage.RoleAssistant:
		if call, ok := message.FirstToolCall(); ok {
			record.Role = repository.RoleToolCall
			record.ToolCallID = call.ID
			record.ToolName = call.Name
			record.ToolArguments = call.Arguments
		} else {
			record.Role = repository.RoleAssistant
			record.Content = message.Text()
		}
	case domainmessage.RoleTool:
		record.Role = repository.RoleTool
		if result, ok := message.FirstToolResult(); ok {
			record.Content = result.Content
			record.ToolCallID = result.ToolCallID
			record.ToolName = result.Name
		} else {
			record.Content = message.Text()
		}
	default:
		return repository.MessageRecord{}, false
	}
	record.ID = m.newID()
	return record, true
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
