package service

import (
	"context"
	"errors"
	"strings"

	loadapi "myai/core/application/session/load/api"
	loadcommand "myai/core/application/session/load/command"
	loadport "myai/core/application/session/load/port"
	"myai/core/session"
)

type LoadService struct {
	// Memory 是运行时事实来源；未命中时才从 Session/Message 仓库重建完整聚合根。
	Memory   loadport.MemoryStore
	Sessions loadport.SessionRecordGetter
	Messages loadport.MessageRecordLister
}

var _ loadapi.Service = LoadService{}

func (s LoadService) Load(ctx context.Context, sessionID string) (*session.Session, error) {
	return s.EnsureInMemory(ctx, loadcommand.EnsureInMemory{SessionID: sessionID})
}

func (s LoadService) LoadCurrent(ctx context.Context, sessionID string) (*session.Session, error) {
	return s.EnsureInMemory(ctx, loadcommand.EnsureInMemory{SessionID: sessionID, SetCurrent: true})
}

func (s LoadService) EnsureInMemory(ctx context.Context, command loadcommand.EnsureInMemory) (*session.Session, error) {
	sessionID := strings.TrimSpace(command.SessionID)
	if sessionID == "" {
		return nil, errors.New("session id is empty")
	}
	if s.Memory == nil {
		return nil, errors.New("session memory store is nil")
	}

	// 已在内存中的会话直接复用，SetCurrent 仅切换当前指针，不重复访问数据库。
	current, err := s.Memory.GetSession(sessionID)
	if err == nil {
		if command.SetCurrent {
			if err := s.Memory.UseSession(sessionID); err != nil {
				return nil, err
			}
		}
		return current, nil
	}
	if s.Sessions == nil {
		return nil, errors.New("session repository is nil")
	}
	if s.Messages == nil {
		return nil, errors.New("message repository is nil")
	}

	// 持久层把 Session 元数据和消息分开保存，加载时在这里重新组装。
	record, err := s.Sessions.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	messageRecords, err := s.Messages.ListMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	messages := MessagesFromRecords(messageRecords)

	err = s.Memory.PutSessionState(session.InitialState{
		ID:                 record.ID,
		Model:              record.Model,
		AgentMode:          session.AgentMode(record.AgentMode),
		PermissionMode:     session.PermissionMode(record.PermissionMode),
		ContextWindowK:     record.ContextWindowK,
		Summary:            record.Summary,
		CompactedMessages:  record.CompactedMessages,
		Usage:              TokenUsageFromRecord(record.Usage),
		LastUsage:          TokenUsageFromRecord(record.LastUsage),
		RAGSettings:        record.RAGSettings,
		GenerationSettings: record.GenerationSettings,
		StyleInstruction:   record.StyleInstruction,
		Messages:           messages,
	}, command.SetCurrent)
	if err != nil {
		return nil, err
	}
	if err := s.Memory.SetCurrentPlanForSession(record.ID, record.CurrentPlan); err != nil {
		return nil, err
	}
	return s.Memory.GetSession(record.ID)
}
