package redis

import (
	"context"
	"encoding/json"
	"errors"

	goredis "github.com/redis/go-redis/v9"
)

func (t *Template) HashSet(ctx context.Context, key string, values map[string]string) error {
	if err := t.verifyClient(); err != nil {
		return err
	}
	if len(values) == 0 {
		return nil
	}
	return t.client.HSet(ctx, key, values).Err()
}

func (t *Template) HashGet(ctx context.Context, key string, field string) (string, bool, error) {
	if err := t.verifyClient(); err != nil {
		return "", false, err
	}
	value, err := t.client.HGet(ctx, key, field).Result()
	if errors.Is(err, goredis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (t *Template) HashGetAll(ctx context.Context, key string) (map[string]string, error) {
	if err := t.verifyClient(); err != nil {
		return nil, err
	}
	return t.client.HGetAll(ctx, key).Result()
}

func (t *Template) HashDelete(ctx context.Context, key string, fields ...string) error {
	if err := t.verifyClient(); err != nil {
		return err
	}
	if len(fields) == 0 {
		return nil
	}
	return t.client.HDel(ctx, key, fields...).Err()
}

func (t *Template) HashSetJSON(ctx context.Context, key string, field string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return t.HashSet(ctx, key, map[string]string{field: string(payload)})
}

func (t *Template) HashGetJSON(ctx context.Context, key string, field string, out any) (bool, error) {
	if out == nil {
		return false, errors.New("redis hash json output is nil")
	}
	payload, found, err := t.HashGet(ctx, key, field)
	if err != nil || !found {
		return found, err
	}
	if err := json.Unmarshal([]byte(payload), out); err != nil {
		return false, err
	}
	return true, nil
}
