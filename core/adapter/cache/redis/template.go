package redis

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type Template struct {
	// Template 统一处理客户端校验、redis.Nil 语义和 JSON 编解码，供具体缓存对象复用。
	client *goredis.Client
}

func NewTemplate(client *goredis.Client) *Template {
	return &Template{client: client}
}

func (t *Template) SetString(ctx context.Context, key string, value string, ttl time.Duration) error {
	if err := t.verifyClient(); err != nil {
		return err
	}
	return t.client.Set(ctx, key, value, ttl).Err()
}

func (t *Template) GetString(ctx context.Context, key string) (string, error) {
	if err := t.verifyClient(); err != nil {
		return "", err
	}

	value, err := t.client.Get(ctx, key).Result()
	// Redis 未命中属于正常状态，统一返回空值而不是把 redis.Nil 暴露给应用层。
	if errors.Is(err, goredis.Nil) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

func (t *Template) Delete(ctx context.Context, key string) error {
	if err := t.verifyClient(); err != nil {
		return err
	}
	return t.client.Del(ctx, key).Err()
}

func (t *Template) Exists(ctx context.Context, key string) (bool, error) {
	if err := t.verifyClient(); err != nil {
		return false, err
	}

	count, err := t.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (t *Template) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return t.SetString(ctx, key, string(payload), ttl)
}

func (t *Template) GetJSON(ctx context.Context, key string, out any) (bool, error) {
	if out == nil {
		return false, errors.New("redis json output is nil")
	}
	if err := t.verifyClient(); err != nil {
		return false, err
	}

	payload, err := t.client.Get(ctx, key).Bytes()
	if errors.Is(err, goredis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return false, err
	}
	return true, nil
}

func (t *Template) SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error) {
	if err := t.verifyClient(); err != nil {
		return false, err
	}
	return t.client.SetNX(ctx, key, value, ttl).Result()
}

func (t *Template) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if err := t.verifyClient(); err != nil {
		return err
	}
	return t.client.Expire(ctx, key, ttl).Err()
}

func (t *Template) TTL(ctx context.Context, key string) (time.Duration, error) {
	if err := t.verifyClient(); err != nil {
		return 0, err
	}
	return t.client.TTL(ctx, key).Result()
}

func (t *Template) verifyClient() error {
	if t == nil || t.client == nil {
		return errors.New("redis client is nil")
	}
	return nil
}
