package redisutil

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type Lock struct {
	client *Client
	key    string
	token  string
}

var releaseLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

func (l *Lock) Release(ctx context.Context) error {
	if l == nil || l.client == nil {
		return nil
	}
	_, err := l.client.RunScript(ctx, releaseLockScript, []string{l.key}, l.token)
	return err
}
