package redis

import (
	"context"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type CurrentSessionCache struct {
	// 该对象只维护 user -> current session ID 映射，不缓存完整会话内容。
	template *Template
}

func NewCurrentSessionCache(client *goredis.Client) *CurrentSessionCache {
	return NewCurrentSessionCacheWithTemplate(NewTemplate(client))
}

func NewCurrentSessionCacheWithTemplate(template *Template) *CurrentSessionCache {
	return &CurrentSessionCache{template: template}
}

func (c *CurrentSessionCache) SetCurrentSession(ctx context.Context, userID string, sessionID string, ttl time.Duration) error {
	return c.template.SetString(ctx, currentSessionKey(userID), sessionID, ttl)
}

func (c *CurrentSessionCache) GetCurrentSession(ctx context.Context, userID string) (string, error) {
	return c.template.GetString(ctx, currentSessionKey(userID))
}

func (c *CurrentSessionCache) DeleteCurrentSession(ctx context.Context, userID string) error {
	return c.template.Delete(ctx, currentSessionKey(userID))
}
