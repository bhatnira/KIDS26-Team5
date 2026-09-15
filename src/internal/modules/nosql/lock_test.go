package nosql

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// fakeLockRedis is a minimal LockRedis used to exercise RunOnce's branching.
type fakeLockRedis struct {
	setnxVal  bool
	setnxErr  error
	existsVal int64
	existsErr error

	setnxCalls  atomic.Int32
	existsCalls atomic.Int32
	evalCalls   atomic.Int32
}

func (f *fakeLockRedis) SetNX(_ context.Context, _ string, _ any, _ time.Duration) *redis.BoolCmd {
	f.setnxCalls.Add(1)
	return redis.NewBoolResult(f.setnxVal, f.setnxErr)
}

func (f *fakeLockRedis) Exists(_ context.Context, _ ...string) *redis.IntCmd {
	f.existsCalls.Add(1)
	return redis.NewIntResult(f.existsVal, f.existsErr)
}

func (f *fakeLockRedis) Eval(_ context.Context, _ string, _ []string, _ ...any) *redis.Cmd {
	f.evalCalls.Add(1)
	return redis.NewCmdResult(int64(1), nil)
}

func testOpts() LockOptions {
	return LockOptions{Key: "k", TTL: time.Second, WaitTimeout: time.Second, Poll: time.Millisecond}
}

func TestRunOnce_WinnerRunsFnAndReleases(t *testing.T) {
	f := &fakeLockRedis{setnxVal: true}
	var fnRuns, verifyRuns int

	err := RunOnce(context.Background(), f, testOpts(),
		func() error { fnRuns++; return nil },
		func() error { verifyRuns++; return nil },
	)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if fnRuns != 1 {
		t.Errorf("fn runs = %d, want 1", fnRuns)
	}
	if verifyRuns != 0 {
		t.Errorf("verify runs = %d, want 0 (winner must not verify)", verifyRuns)
	}
	if f.evalCalls.Load() != 1 {
		t.Errorf("release (Eval) calls = %d, want 1", f.evalCalls.Load())
	}
}

func TestRunOnce_LoserWaitsThenVerifies(t *testing.T) {
	// Lost the SETNX race; the lock is already gone (winner finished).
	f := &fakeLockRedis{setnxVal: false, existsVal: 0}
	var fnRuns, verifyRuns int

	err := RunOnce(context.Background(), f, testOpts(),
		func() error { fnRuns++; return nil },
		func() error { verifyRuns++; return nil },
	)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if fnRuns != 0 {
		t.Errorf("fn runs = %d, want 0 (loser must not run fn)", fnRuns)
	}
	if verifyRuns != 1 {
		t.Errorf("verify runs = %d, want 1", verifyRuns)
	}
}

func TestRunOnce_LoserWithNilVerifySkips(t *testing.T) {
	f := &fakeLockRedis{setnxVal: false, existsVal: 0}
	var fnRuns int

	err := RunOnce(context.Background(), f, testOpts(),
		func() error { fnRuns++; return nil }, nil)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if fnRuns != 0 {
		t.Errorf("fn runs = %d, want 0", fnRuns)
	}
}

func TestRunOnce_RedisErrorFallsBackToFn(t *testing.T) {
	f := &fakeLockRedis{setnxErr: errors.New("redis down")}
	var fnRuns int

	err := RunOnce(context.Background(), f, testOpts(),
		func() error { fnRuns++; return nil }, nil)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if fnRuns != 1 {
		t.Errorf("fn runs = %d, want 1 (best-effort fallback)", fnRuns)
	}
}

func TestRunOnce_LoserTimesOutWhenLockNeverClears(t *testing.T) {
	// Lost the race and the lock never disappears → wait must time out.
	f := &fakeLockRedis{setnxVal: false, existsVal: 1}
	opts := LockOptions{Key: "k", TTL: time.Second, WaitTimeout: 20 * time.Millisecond, Poll: time.Millisecond}

	err := RunOnce(context.Background(), f, opts,
		func() error { return nil },
		func() error { return nil },
	)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}
