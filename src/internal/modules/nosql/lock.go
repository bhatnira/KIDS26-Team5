package nosql

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"antelope/internal/modules/log"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// LockRedis is the subset of redis.UniversalClient that the lock helpers need.
// Accepting it (rather than the full client) keeps RunOnce unit-testable with a
// lightweight fake; a real *redis.Client / redis.UniversalClient satisfies it.
type LockRedis interface {
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd
	Exists(ctx context.Context, keys ...string) *redis.IntCmd
	Eval(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd
}

// LockOptions configures a RunOnce distributed lock.
type LockOptions struct {
	// Key is the Redis key holding the lock.
	Key string
	// TTL is the lock expiry; it must comfortably exceed how long fn takes so
	// the lock never expires out from under the winner mid-run.
	TTL time.Duration
	// WaitTimeout bounds how long a non-winner waits for the winner to finish.
	WaitTimeout time.Duration
	// Poll is the interval at which a non-winner checks whether the lock cleared.
	Poll time.Duration
}

// releaseLockScript deletes the key only if we still own it (owner-checked,
// so an expired-then-reacquired lock held by another process is never deleted).
const releaseLockScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
else
    return 0
end`

// RunOnce executes fn under a Redis lock so that, across all processes
// contending on opts.Key, only one runs fn at a time. It is intended for
// startup work that must not run concurrently across pods — schema migration
// being the canonical case.
//
//   - The winner (atomic SETNX) runs fn and releases the lock afterwards.
//   - A non-winner polls until the lock disappears (the winner finished), then
//     runs verify (if non-nil) to confirm/repair the result. verify covers the
//     case where the winner crashed mid-fn and the lock merely expired: pass a
//     verify that re-runs the (idempotent) work, or one that checks the result
//     and only repairs when needed. If verify is nil the non-winner returns nil
//     without running fn.
//   - If Redis is unreachable RunOnce degrades to running fn unguarded
//     (best-effort), matching the project's existing migration posture.
func RunOnce(ctx context.Context, rdb LockRedis, opts LockOptions, fn, verify func() error) error {
	owner, err := randOwner()
	if err != nil {
		return fmt.Errorf("nosql: generate lock owner: %w", err)
	}

	acquired, err := rdb.SetNX(ctx, opts.Key, owner, opts.TTL).Result()
	if err != nil {
		// Best-effort guard, not a hard dependency: proceed without the lock.
		log.L().Warn("could not acquire Redis lock; proceeding without it",
			zap.String("key", opts.Key))
		return fn()
	}
	if acquired {
		defer releaseLock(rdb, opts.Key, owner)
		return fn()
	}

	// Another process is running fn — wait for it to finish.
	if err := waitForRelease(ctx, rdb, opts); err != nil {
		return err
	}
	if verify != nil {
		return verify()
	}
	return nil
}

// waitForRelease polls until the lock key disappears or the wait deadline passes.
func waitForRelease(ctx context.Context, rdb LockRedis, opts LockOptions) error {
	deadline := time.Now().Add(opts.WaitTimeout)
	ticker := time.NewTicker(opts.Poll)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("nosql: timed out waiting for lock %q", opts.Key)
			}
			exists, err := rdb.Exists(ctx, opts.Key).Result()
			if err != nil {
				continue // transient Redis blip — keep waiting
			}
			if exists == 0 {
				return nil
			}
		}
	}
}

// releaseLock deletes the lock key only if this owner still holds it.
func releaseLock(rdb LockRedis, key, owner string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := rdb.Eval(ctx, releaseLockScript, []string{key}, owner).Result(); err != nil {
		log.L().Warn("could not release Redis lock (may have already expired)", zap.String("key", key))
	}
}

func randOwner() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
