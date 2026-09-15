package session

import (
	"context"
	"sync"
	"time"
)

const gcInterval = 5 * time.Minute

// MemoryStore is a thread-safe in-memory fallback.
// The background GC goroutine exits cleanly when ctx is cancelled,
// so callers should pass the application-level context to avoid goroutine leaks.
//
// Example:
//
//	store := session.NewMemoryStore(appCtx)
type MemoryStore struct {
	mu      sync.RWMutex
	entries map[string]time.Time
	epochs  map[uint]int64
}

func NewMemoryStore(ctx context.Context) *MemoryStore {
	s := &MemoryStore{
		entries: make(map[string]time.Time),
		epochs:  make(map[uint]int64),
	}
	go s.gc(ctx)
	return s
}

func (s *MemoryStore) Revoke(_ context.Context, jti string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[jti] = time.Now().Add(ttl)
	return nil
}

func (s *MemoryStore) IsRevoked(_ context.Context, jti string) bool {
	s.mu.RLock()
	exp, ok := s.entries[jti]
	s.mu.RUnlock()

	if !ok {
		return false
	}
	if time.Now().After(exp) {
		s.mu.Lock()
		delete(s.entries, jti)
		s.mu.Unlock()
		return false
	}
	return true
}

func (s *MemoryStore) CurrentEpoch(_ context.Context, userID uint) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.epochs[userID], nil
}

func (s *MemoryStore) BumpUserEpoch(_ context.Context, userID uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.epochs[userID]++
	return nil
}

func (s *MemoryStore) gc(ctx context.Context) {
	ticker := time.NewTicker(gcInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweep()
		}
	}
}

func (s *MemoryStore) sweep() {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for jti, exp := range s.entries {
		if now.After(exp) {
			delete(s.entries, jti)
		}
	}
}
