package service

import (
	"context"
	"errors"
	"strings"
	"time"

	lifecyclecommand "myai/core/application/session/lifecycle/command"
	lifecycleport "myai/core/application/session/lifecycle/port"
	lifecycleresult "myai/core/application/session/lifecycle/result"
	"myai/core/session"
)

type LifecycleService struct {
	// LifecycleService 处理内存和仓库的原子操作；跨用例副作用由外层 UseCase 编排。
	Memory   lifecycleport.MemoryStore
	Loader   lifecycleport.SessionLoader
	Sessions lifecycleport.SessionRepository
	Messages lifecycleport.MessageRepository
	Now      func() time.Time
}

var _ lifecycleport.LifecycleService = LifecycleService{}

func (s LifecycleService) NewSession(ctx context.Context) (*session.Session, error) {
	if s.Memory == nil {
		return nil, errors.New("session manager is nil")
	}
	if err := s.Memory.NewSession(); err != nil {
		return nil, err
	}
	return s.Memory.Current()
}

func (s LifecycleService) LoadSession(ctx context.Context, sessionID string) (*session.Session, error) {
	if s.Loader != nil {
		return s.Loader.LoadCurrent(ctx, sessionID)
	}
	if s.Memory == nil {
		return nil, errors.New("session manager is nil")
	}
	current, err := s.Memory.GetSession(strings.TrimSpace(sessionID))
	if err != nil {
		return nil, errors.New("session loader is nil")
	}
	if err := s.Memory.UseSession(current.ID); err != nil {
		return nil, err
	}
	return current, nil
}

func (s LifecycleService) DeleteSession(ctx context.Context, command lifecyclecommand.DeleteSession) (lifecycleresult.DeleteSession, error) {
	if s.Memory == nil {
		return lifecycleresult.DeleteSession{}, errors.New("session manager is nil")
	}

	sessionID := strings.TrimSpace(command.SessionID)
	if sessionID == "" {
		sessionID = s.Memory.CurrentSessionId()
	}
	if sessionID == "" {
		return lifecycleresult.DeleteSession{}, errors.New("session id is empty")
	}

	// 先软删除持久化记录，再移除内存对象，避免数据库失败后会话在进程内提前消失。
	deletingCurrent := sessionID == s.Memory.CurrentSessionId()
	if s.Sessions != nil {
		if _, err := s.Sessions.GetSession(ctx, sessionID); err != nil {
			return lifecycleresult.DeleteSession{}, err
		}
		if err := s.Sessions.MarkSessionDeleted(ctx, sessionID, s.now()); err != nil {
			return lifecycleresult.DeleteSession{}, err
		}
	}
	if err := s.Memory.RemoveSession(sessionID); err != nil {
		return lifecycleresult.DeleteSession{}, err
	}
	return lifecycleresult.DeleteSession{
		SessionID:      sessionID,
		DeletedCurrent: deletingCurrent,
	}, nil
}

func (s LifecycleService) RestoreSession(ctx context.Context, command lifecyclecommand.RestoreSession) (string, error) {
	sessionID := strings.TrimSpace(command.SessionID)
	if sessionID == "" {
		return "", errors.New("session id is empty")
	}
	if s.Sessions == nil {
		return "", errors.New("store is nil")
	}
	if err := s.Sessions.MarkSessionRestored(ctx, sessionID, s.now()); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (s LifecycleService) ClearCurrent(ctx context.Context) (*session.Session, error) {
	if s.Memory == nil {
		return nil, errors.New("session manager is nil")
	}

	current, err := s.Memory.Current()
	if err != nil {
		return nil, err
	}
	// Clear 与 Delete 不同：保留 Session ID 和设置，只清空消息、摘要、用量和 Plan。
	if s.Messages != nil {
		if err := s.Messages.ClearMessages(ctx, current.ID); err != nil {
			return nil, err
		}
	}
	if err := s.Memory.ClearCurrent(); err != nil {
		return nil, err
	}
	return current, nil
}

func (s LifecycleService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
