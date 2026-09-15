package session

import (
	"testing"
)

func TestMemoryStore_Epoch(t *testing.T) {
	ctx := t.Context()
	s := NewMemoryStore(ctx)

	// Default epoch is 0 so pre-existing (epoch-0) tokens stay valid.
	if e, err := s.CurrentEpoch(ctx, 42); err != nil || e != 0 {
		t.Fatalf("default epoch = (%d,%v), want (0,nil)", e, err)
	}

	// Bumping advances the epoch monotonically.
	for want := int64(1); want <= 3; want++ {
		if err := s.BumpUserEpoch(ctx, 42); err != nil {
			t.Fatalf("BumpUserEpoch: %v", err)
		}
		if e, _ := s.CurrentEpoch(ctx, 42); e != want {
			t.Fatalf("epoch after %d bumps = %d, want %d", want, e, want)
		}
	}

	// Epochs are per-user.
	if e, _ := s.CurrentEpoch(ctx, 99); e != 0 {
		t.Fatalf("unrelated user epoch = %d, want 0", e)
	}
}

// TestEpochInvalidation models the AuthMiddleware decision: a token minted at an
// epoch lower than the user's current epoch is rejected; one minted at/after it
// is accepted.
func TestEpochInvalidation(t *testing.T) {
	ctx := t.Context()
	s := NewMemoryStore(ctx)

	tokenEpoch, _ := s.CurrentEpoch(ctx, 7) // minted now (=0)

	cur, _ := s.CurrentEpoch(ctx, 7)
	if tokenEpoch < cur {
		t.Fatal("token should be valid before any bump")
	}

	_ = s.BumpUserEpoch(ctx, 7) // role change / disable / password reset

	cur, _ = s.CurrentEpoch(ctx, 7)
	if !(tokenEpoch < cur) {
		t.Fatal("token should be force-revoked after a bump")
	}

	// A freshly minted token carries the new epoch and is valid again.
	newToken, _ := s.CurrentEpoch(ctx, 7)
	if newToken < cur {
		t.Fatal("re-minted token should be valid")
	}
}
