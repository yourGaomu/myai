package redis

import (
	"context"
	"errors"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
)

func (t *Template) SortedSetAdd(ctx context.Context, key string, members ...SortedSetMember) error {
	if err := t.verifyClient(); err != nil {
		return err
	}
	if len(members) == 0 {
		return nil
	}
	values := make([]goredis.Z, 0, len(members))
	for _, member := range members {
		values = append(values, goredis.Z{Score: member.Score, Member: member.Value})
	}
	return t.client.ZAdd(ctx, key, values...).Err()
}

func (t *Template) SortedSetRange(ctx context.Context, key string, start int64, stop int64, reverse bool) ([]SortedSetMember, error) {
	if err := t.verifyClient(); err != nil {
		return nil, err
	}
	var (
		values []goredis.Z
		err    error
	)
	if reverse {
		values, err = t.client.ZRevRangeWithScores(ctx, key, start, stop).Result()
	} else {
		values, err = t.client.ZRangeWithScores(ctx, key, start, stop).Result()
	}
	if err != nil {
		return nil, err
	}
	return sortedSetMembers(values), nil
}

func (t *Template) SortedSetRangeByScore(ctx context.Context, key string, scoreRange ScoreRange) ([]SortedSetMember, error) {
	if err := t.verifyClient(); err != nil {
		return nil, err
	}
	query := &goredis.ZRangeBy{
		Min:    scoreRange.Min,
		Max:    scoreRange.Max,
		Offset: scoreRange.Offset,
		Count:  scoreRange.Count,
	}
	var (
		values []goredis.Z
		err    error
	)
	if scoreRange.Reverse {
		values, err = t.client.ZRevRangeByScoreWithScores(ctx, key, query).Result()
	} else {
		values, err = t.client.ZRangeByScoreWithScores(ctx, key, query).Result()
	}
	if err != nil {
		return nil, err
	}
	return sortedSetMembers(values), nil
}

func (t *Template) SortedSetRemove(ctx context.Context, key string, values ...string) error {
	if err := t.verifyClient(); err != nil {
		return err
	}
	if len(values) == 0 {
		return nil
	}
	members := make([]any, 0, len(values))
	for _, value := range values {
		members = append(members, value)
	}
	return t.client.ZRem(ctx, key, members...).Err()
}

func (t *Template) SortedSetScore(ctx context.Context, key string, value string) (float64, bool, error) {
	if err := t.verifyClient(); err != nil {
		return 0, false, err
	}
	score, err := t.client.ZScore(ctx, key, value).Result()
	if errors.Is(err, goredis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return score, true, nil
}

func sortedSetMembers(values []goredis.Z) []SortedSetMember {
	members := make([]SortedSetMember, 0, len(values))
	for _, value := range values {
		member, ok := value.Member.(string)
		if !ok {
			member = fmt.Sprint(value.Member)
		}
		members = append(members, SortedSetMember{Value: member, Score: value.Score})
	}
	return members
}
