package redisutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	client redis.UniversalClient
}

func New(client redis.UniversalClient) *Client {
	return &Client{client: client}
}

func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, key).Result()
}

func (c *Client) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *Client) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return c.client.HGetAll(ctx, key).Result()
}

func (c *Client) HSet(ctx context.Context, key string, values map[string]interface{}) error {
	return c.client.HSet(ctx, key, values).Err()
}

func (c *Client) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.client.Expire(ctx, key, ttl).Err()
}

func (c *Client) Del(ctx context.Context, keys ...string) error {
	return c.client.Del(ctx, keys...).Err()
}

func (c *Client) RunScript(ctx context.Context, script *redis.Script, keys []string, args ...interface{}) (interface{}, error) {
	return script.Run(ctx, c.client, keys, args...).Result()
}

func (c *Client) Close() error {
	return c.client.Close()
}

func (c *Client) AcquireLock(ctx context.Context, key string, ttl time.Duration) (*Lock, bool, error) {
	token, err := randomToken()
	if err != nil {
		return nil, false, err
	}
	ok, err := c.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil || !ok {
		return nil, ok, err
	}
	return &Lock{
		client: c,
		key:    key,
		token:  token,
	}, true, nil
}

func randomToken() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(data[:]), nil
}
