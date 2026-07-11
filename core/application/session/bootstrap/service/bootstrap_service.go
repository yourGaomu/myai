package service

import (
	"context"
	"errors"
	"strings"

	bootstrapapi "myai/core/application/session/bootstrap/api"
	bootstrapcommand "myai/core/application/session/bootstrap/command"
	bootstrapport "myai/core/application/session/bootstrap/port"
	bootstrapresult "myai/core/application/session/bootstrap/result"
	sessioncommand "myai/core/application/session/command"
	repository "myai/core/port/repository"
	"myai/core/session"
)

type BootstrapService struct {
	// 启动恢复顺序：Redis 当前会话指针 -> 内存当前会话 -> 创建新会话。
	Cache       bootstrapport.Cache
	Lifecycle   bootstrapport.Lifecycle
	State       bootstrapport.State
	Persistence bootstrapport.Persistence
}

var _ bootstrapapi.Service = BootstrapService{}

func (s BootstrapService) Bootstrap(ctx context.Context, command bootstrapcommand.Bootstrap) (bootstrapresult.Bootstrap, error) {
	if s.Lifecycle == nil || s.State == nil {
		return bootstrapresult.Bootstrap{}, errors.New("session manager is nil")
	}

	// Redis 只保存会话 ID；真实 Session 和消息仍通过 Lifecycle 从仓库恢复到内存。
	if cachedID, err := s.cachedSessionID(ctx); err != nil {
		return bootstrapresult.Bootstrap{}, err
	} else if cachedID != "" {
		current, err := s.Lifecycle.LoadSession(ctx, cachedID)
		if err == nil {
			if err := s.saveCurrentSession(ctx, current.ID); err != nil {
				return bootstrapresult.Bootstrap{}, err
			}
			return bootstrapresult.Bootstrap{Session: current, Action: bootstrapresult.ActionLoaded}, nil
		}
		if !errors.Is(err, repository.ErrNotFound) {
			return bootstrapresult.Bootstrap{}, err
		}
	}

	current, err := s.State.CurrentSession()
	if err != nil || current == nil {
		return s.createSession(ctx, command)
	}
	if err := s.persistSession(ctx, current, command.NewSessionTitle); err != nil {
		return bootstrapresult.Bootstrap{}, err
	}
	return bootstrapresult.Bootstrap{Session: current, Action: bootstrapresult.ActionReused}, nil
}

func (s BootstrapService) createSession(ctx context.Context, command bootstrapcommand.Bootstrap) (bootstrapresult.Bootstrap, error) {
	current, err := s.Lifecycle.NewSession(ctx)
	if err != nil {
		return bootstrapresult.Bootstrap{}, err
	}
	if err := s.persistSession(ctx, current, command.NewSessionTitle); err != nil {
		return bootstrapresult.Bootstrap{}, err
	}
	if err := s.saveCurrentSession(ctx, current.ID); err != nil {
		return bootstrapresult.Bootstrap{}, err
	}
	return bootstrapresult.Bootstrap{Session: current, Action: bootstrapresult.ActionCreated}, nil
}

func (s BootstrapService) persistSession(ctx context.Context, current *session.Session, title string) error {
	if s.Persistence == nil {
		return nil
	}
	return s.Persistence.Save(ctx, sessioncommand.SaveSession{
		SessionID: current.ID,
		Model:     current.Model,
		Title:     strings.TrimSpace(title),
	})
}

func (s BootstrapService) cachedSessionID(ctx context.Context) (string, error) {
	if s.Cache == nil {
		return "", nil
	}
	sessionID, err := s.Cache.Get(ctx)
	return strings.TrimSpace(sessionID), err
}

func (s BootstrapService) saveCurrentSession(ctx context.Context, sessionID string) error {
	if s.Cache == nil {
		return nil
	}
	return s.Cache.Save(ctx, sessionID)
}
