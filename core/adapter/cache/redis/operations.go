package redis

import (
	"context"
	"time"
)

type StringOperations interface {
	SetString(ctx context.Context, key string, value string, ttl time.Duration) error
	GetString(ctx context.Context, key string) (string, error)
	SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error
	GetJSON(ctx context.Context, key string, out any) (bool, error)
	SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error)
}

type HashOperations interface {
	HashSet(ctx context.Context, key string, values map[string]string) error
	HashGet(ctx context.Context, key string, field string) (string, bool, error)
	HashGetAll(ctx context.Context, key string) (map[string]string, error)
	HashDelete(ctx context.Context, key string, fields ...string) error
	HashSetJSON(ctx context.Context, key string, field string, value any) error
	HashGetJSON(ctx context.Context, key string, field string, out any) (bool, error)
}

type SortedSetOperations interface {
	SortedSetAdd(ctx context.Context, key string, members ...SortedSetMember) error
	SortedSetRange(ctx context.Context, key string, start int64, stop int64, reverse bool) ([]SortedSetMember, error)
	SortedSetRangeByScore(ctx context.Context, key string, scoreRange ScoreRange) ([]SortedSetMember, error)
	SortedSetRemove(ctx context.Context, key string, values ...string) error
	SortedSetScore(ctx context.Context, key string, value string) (float64, bool, error)
}

type KeyOperations interface {
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	TTL(ctx context.Context, key string) (time.Duration, error)
}

type Operations interface {
	// Operations 类似 Java RedisTemplate 的受控能力集合，业务对象不直接依赖 go-redis Client。
	StringOperations
	HashOperations
	SortedSetOperations
	KeyOperations
}
