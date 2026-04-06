package valkey

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
	"github.com/xiaocaoooo/gallery-server/internal/config"
)

var errLockNotHeld = errors.New("lock not held")

var releaseScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
  return redis.call("del", KEYS[1])
end
return 0
`)

type Locker struct {
	client *redis.Client
	ttl    time.Duration
}

func New(ctx context.Context, cfg config.ValkeyConfig) (*Locker, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping valkey: %w", err)
	}

	return &Locker{client: client, ttl: cfg.LockTTL}, nil
}

func (l *Locker) Acquire(ctx context.Context, key string) (string, error) {
	token := uuid.NewString()
	ok, err := l.client.SetNX(ctx, key, token, l.ttl).Result()
	if err != nil {
		return "", fmt.Errorf("acquire valkey lock: %w", err)
	}
	if !ok {
		return "", errLockNotHeld
	}
	return token, nil
}

func (l *Locker) Release(ctx context.Context, key, token string) error {
	result, err := releaseScript.Run(ctx, l.client, []string{key}, token).Int()
	if err != nil {
		return fmt.Errorf("release valkey lock: %w", err)
	}
	if result == 0 {
		return errLockNotHeld
	}
	return nil
}

func (l *Locker) Close() error {
	if l == nil || l.client == nil {
		return nil
	}
	return l.client.Close()
}
