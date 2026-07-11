package service

import (
	"context"
	"errors"
	"time"

	sessioncommand "myai/core/application/session/command"
	persistenceapi "myai/core/application/session/persistence/api"
	persistencecommand "myai/core/application/session/persistence/command"
	persistenceport "myai/core/application/session/persistence/port"
	"myai/core/contextmgr"
	"myai/core/llm"
	agentplan "myai/core/plan"
	repository "myai/core/port/repository"
	"myai/core/session"
)

const DefaultTitle = "New chat"

type PersistenceService struct {
	// PersistenceService 把内存 Session 快照转换为仓库 Record，并补齐旧记录和默认值。
	Memory       persistenceport.SnapshotMemory
	Sessions     persistenceport.Repository
	DefaultModel string
	Now          func() time.Time
}

var _ persistenceapi.Service = PersistenceService{}

func (s PersistenceService) Save(ctx context.Context, command sessioncommand.SaveSession) error {
	if s.Sessions == nil {
		return nil
	}

	// 优先从内存取完整运行态；若会话未加载，则保留数据库既有字段避免被零值覆盖。
	current := s.memorySession(command.SessionID)
	var existing repository.SessionRecord
	hasExisting := false
	if current == nil {
		record, err := s.Sessions.GetSession(ctx, command.SessionID)
		if err == nil {
			existing = record
			hasExisting = true
		} else if !errors.Is(err, repository.ErrNotFound) {
			return err
		}
	}

	return s.SaveRecord(ctx, BuildSessionRecord(persistencecommand.BuildRecord{
		SessionID: command.SessionID, Model: command.Model, Title: command.Title,
		DefaultModel: s.DefaultModel, Current: current, Existing: existing,
		HasExisting: hasExisting, Now: s.now(),
	}))
}

func (s PersistenceService) SaveRecord(ctx context.Context, record repository.SessionRecord) error {
	if s.Sessions == nil {
		return nil
	}
	existing, err := s.Sessions.GetSession(ctx, record.ID)
	hasExisting := err == nil
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	record, err = PrepareSessionRecordForSave(persistencecommand.PrepareRecord{
		Record: record, Existing: existing, HasExisting: hasExisting,
		DefaultModel: s.DefaultModel, Now: s.now(),
	})
	if err != nil {
		return err
	}
	return s.Sessions.SaveSession(ctx, record)
}

func (s PersistenceService) memorySession(sessionID string) *session.Session {
	if s.Memory == nil {
		return nil
	}
	current, err := s.Memory.GetSession(sessionID)
	if err != nil || current == nil || current.ID != sessionID {
		return nil
	}
	return current
}

func (s PersistenceService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func BuildSessionRecord(command persistencecommand.BuildRecord) repository.SessionRecord {
	// Record 是应用层与持久化适配器的中立契约，不包含 BSON/JSON 标签。
	now := command.Now
	if now.IsZero() {
		now = time.Now()
	}
	model := command.Model
	if model == "" {
		model = command.DefaultModel
	}
	title := command.Title
	if title == "" {
		title = DefaultTitle
	}
	record := repository.SessionRecord{
		ID: command.SessionID, Model: model, AgentMode: string(session.AgentModeChat),
		PermissionMode: string(session.PermissionModeAsk), ContextWindowK: contextmgr.DefaultWindowK, Title: title,
	}
	if current := command.Current; current != nil && current.ID == command.SessionID {
		record.AgentMode = string(session.NormalizeAgentMode(current.AgentMode))
		record.PermissionMode = string(session.NormalizePermissionMode(current.PermissionMode))
		record.ContextWindowK = contextmgr.NormalizeWindowK(current.ContextWindowK)
		record.Summary = current.Summary
		record.CompactedMessages = current.CompactedMessages
		record.Usage = TokenUsageRecord(current.Usage)
		record.LastUsage = TokenUsageRecord(current.LastUsage)
		record.CurrentPlan = agentplan.Clone(current.CurrentPlan)
		if record.Summary != "" {
			record.CompactedAt = &now
		}
		return record
	}
	if command.HasExisting {
		existing := command.Existing
		if existing.PermissionMode != "" {
			record.PermissionMode = existing.PermissionMode
		}
		if existing.AgentMode != "" {
			record.AgentMode = existing.AgentMode
		}
		if existing.ContextWindowK > 0 {
			record.ContextWindowK = existing.ContextWindowK
		}
		record.Summary = existing.Summary
		record.CompactedMessages = existing.CompactedMessages
		record.CompactedAt = existing.CompactedAt
		record.Usage = existing.Usage
		record.LastUsage = existing.LastUsage
		record.CurrentPlan = agentplan.Clone(existing.CurrentPlan)
	}
	return record
}

func PrepareSessionRecordForSave(command persistencecommand.PrepareRecord) (repository.SessionRecord, error) {
	record := command.Record
	if record.ID == "" {
		return repository.SessionRecord{}, errors.New("session id is empty")
	}
	if record.Model == "" {
		record.Model = command.DefaultModel
	}
	if record.AgentMode == "" {
		record.AgentMode = string(session.AgentModeChat)
	}
	if record.PermissionMode == "" {
		record.PermissionMode = string(session.PermissionModeAsk)
	}
	record.ContextWindowK = contextmgr.NormalizeWindowK(record.ContextWindowK)
	if record.Title == "" {
		record.Title = DefaultTitle
	}
	now := command.Now
	if now.IsZero() {
		now = time.Now()
	}
	if record.CreatedAt.IsZero() && command.HasExisting {
		record.CreatedAt = command.Existing.CreatedAt
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if command.HasExisting && command.Existing.Title != "" && command.Existing.Title != DefaultTitle {
		record.Title = command.Existing.Title
	}
	record.UpdatedAt = now
	return record, nil
}

func TokenUsageRecord(usage llm.TokenUsage) *repository.TokenUsageRecord {
	if !usage.Available && usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 && usage.ReasoningTokens == 0 && usage.PromptCachedTokens == 0 {
		return nil
	}
	return &repository.TokenUsageRecord{
		PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens,
		TotalTokens: usage.TotalTokens, ReasoningTokens: usage.ReasoningTokens,
		PromptCachedTokens: usage.PromptCachedTokens, Available: usage.Available,
	}
}
