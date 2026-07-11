package service

import (
	"context"
	"time"

	currentapi "myai/core/application/session/current/api"
	cacheport "myai/core/port/cache"
)

type SessionService struct {
	// 当前会话缓存按 user 隔离并设置 TTL；它保存的是 ID，不是 Session 本体。
	Cache  cacheport.CurrentSessionCache
	UserID string
	TTL    time.Duration
}

var _ currentapi.SessionService = SessionService{}

func (s SessionService) Get(ctx context.Context) (string, error) {
	if s.Cache == nil {
		return "", nil
	}
	return s.Cache.GetCurrentSession(ctx, s.UserID)
}

func (s SessionService) Save(ctx context.Context, sessionID string) error {
	if s.Cache == nil {
		return nil
	}
	return s.Cache.SetCurrentSession(ctx, s.UserID, sessionID, s.TTL)
}

func (s SessionService) Delete(ctx context.Context) error {
	if s.Cache == nil {
		return nil
	}
	return s.Cache.DeleteCurrentSession(ctx, s.UserID)
}
