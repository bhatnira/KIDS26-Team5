// Package session manages the validity of issued JWT sessions.
// A "session" here corresponds to a single access-token JTI; revoking it
// (e.g. on logout) makes AuthMiddleware reject the token before it naturally
// expires, giving immediate logout semantics without a round-trip to the DB.
//
// Two backends are provided:
//   - RedisStore — preferred in production; uses Redis SET with TTL.
//   - MemoryStore — in-process fallback for single-instance deployments without Redis.
package session

import (
	"context"
	"time"
)

const keyPrefix = "session:revoked:"

// Store is the interface for checking and invalidating token sessions.
type Store interface {
	// Revoke marks the given JTI as invalid for the duration of ttl.
	Revoke(ctx context.Context, jti string, ttl time.Duration) error
	// IsRevoked reports whether the JTI's session has been invalidated.
	IsRevoked(ctx context.Context, jti string) bool

	// CurrentEpoch returns the user's current token epoch. Every token embeds the
	// epoch in effect when it was minted; a token whose epoch is lower than this
	// has been force-invalidated (e.g. by a role/status change or password reset).
	// Returns 0 when the user has no epoch set (the default for never-bumped users,
	// which keeps pre-existing tokens valid).
	CurrentEpoch(ctx context.Context, userID uint) (int64, error)
	// BumpUserEpoch increments the user's token epoch, immediately invalidating
	// every access token previously issued to that user.
	BumpUserEpoch(ctx context.Context, userID uint) error
}

const epochPrefix = "session:epoch:"
