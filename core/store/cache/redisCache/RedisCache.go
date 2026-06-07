package redisCache

import (
	"context"
	"errors"
	"time"

	redis "github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
}

func New(client *redis.Client) *RedisCache {
	return &RedisCache{client: client}
}

func (r *RedisCache) SetCurrentSession(ctx context.Context, userID string, sessionID string, ttl time.Duration) error {
	if err := r.verifyClient(); err != nil {
		return err
	}

	return r.client.Set(ctx, currentSessionKey(userID), sessionID, ttl).Err()
}

func (r *RedisCache) GetCurrentSession(ctx context.Context, userID string) (string, error) {
	if err := r.verifyClient(); err != nil {
		return "", err
	}

	sessionID, err := r.client.Get(ctx, currentSessionKey(userID)).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	return sessionID, nil
}

func (r *RedisCache) DeleteCurrentSession(ctx context.Context, userID string) error {
	if err := r.verifyClient(); err != nil {
		return err
	}

	return r.client.Del(ctx, currentSessionKey(userID)).Err()
}

func (r *RedisCache) verifyClient() error {
	if r.client == nil {
		return errors.New("redis client is nil")
	}
	return nil
}

func currentSessionKey(userID string) string {
	return "myai:current_session:" + userID
}
