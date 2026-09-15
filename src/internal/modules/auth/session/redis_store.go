package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisStore uses Redis SET with TTL for session invalidation.
type RedisStore struct {
	client redis.UniversalClient
}

func NewRedisStore(client redis.UniversalClient) *RedisStore {
	return &RedisStore{client: client}
}

func (s *RedisStore) Revoke(ctx context.Context, jti string, ttl time.Duration) error {
	return s.client.Set(ctx, keyPrefix+jti, 1, ttl).Err()
}

func (s *RedisStore) IsRevoked(ctx context.Context, jti string) bool {
	val, err := s.client.Exists(ctx, keyPrefix+jti).Result()
	if err != nil {
		zap.L().Warn("redis session store check failed", zap.Error(err))
		return false
	}
	return val > 0
}

func (s *RedisStore) CurrentEpoch(ctx context.Context, userID uint) (int64, error) {
	v, err := s.client.Get(ctx, epochKey(userID)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return v, nil
}

func (s *RedisStore) BumpUserEpoch(ctx context.Context, userID uint) error {
	// INCR is atomic and creates the key at 1 on first use, so it is correct even
	// under concurrent bumps from multiple pods.
	return s.client.Incr(ctx, epochKey(userID)).Err()
}

func epochKey(userID uint) string { return fmt.Sprintf("%s%d", epochPrefix, userID) }
