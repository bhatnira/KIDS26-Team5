package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tooliface "trpc.group/trpc-go/trpc-agent-go/tool"
)

// fakeToolSet is a stand-in for a live MCP toolset that records whether Close
// was called, so tests can assert the pool releases evicted connections.
type fakeToolSet struct {
	name   string
	closed atomic.Bool
}

func (f *fakeToolSet) Name() string                           { return f.name }
func (f *fakeToolSet) Tools(context.Context) []tooliface.Tool { return nil }
func (f *fakeToolSet) Close() error                           { f.closed.Store(true); return nil }

// newTestPool builds a pool WITHOUT starting the background sweeper goroutine,
// so tests drive eviction deterministically.
func newTestPool(maxEntries int) *MCPPool {
	return &MCPPool{
		maxEntries: maxEntries,
		cache:      make(map[uint]*userBundle),
		done:       make(chan struct{}),
	}
}

func (p *MCPPool) sizeForTest() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.cache)
}

func (p *MCPPool) hasForTest(userID uint) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.cache[userID]
	return ok
}

func TestInsertEvictsLRUWhenFull(t *testing.T) {
	p := newTestPool(3)
	a := &fakeToolSet{name: "a"}
	b := &fakeToolSet{name: "b"}
	c := &fakeToolSet{name: "c"}
	d := &fakeToolSet{name: "d"}

	p.insert(1, "h1", []tooliface.ToolSet{a})
	p.insert(2, "h2", []tooliface.ToolSet{b})
	p.insert(3, "h3", []tooliface.ToolSet{c})

	// Make user 1 the least-recently-used.
	now := time.Now()
	p.mu.Lock()
	p.cache[1].lastUsedAt = now.Add(-3 * time.Minute)
	p.cache[2].lastUsedAt = now.Add(-2 * time.Minute)
	p.cache[3].lastUsedAt = now.Add(-1 * time.Minute)
	p.mu.Unlock()

	p.insert(4, "h4", []tooliface.ToolSet{d}) // exceeds cap of 3

	if got := p.sizeForTest(); got != 3 {
		t.Fatalf("cache size = %d, want 3 (cap)", got)
	}
	if p.hasForTest(1) {
		t.Fatal("user 1 (LRU) should have been evicted")
	}
	if !a.closed.Load() {
		t.Fatal("evicted toolset should be Closed")
	}
	for _, id := range []uint{2, 3, 4} {
		if !p.hasForTest(id) {
			t.Fatalf("user %d should remain cached", id)
		}
	}
}

func TestInsertReplaceClosesOldNoGrowth(t *testing.T) {
	p := newTestPool(2)
	old := &fakeToolSet{name: "old"}
	updated := &fakeToolSet{name: "new"}

	p.insert(1, "h1", []tooliface.ToolSet{old})
	p.insert(1, "h2", []tooliface.ToolSet{updated}) // same user, new hash

	if !old.closed.Load() {
		t.Fatal("old toolset should be Closed on replace")
	}
	if updated.closed.Load() {
		t.Fatal("new toolset should not be Closed")
	}
	if got := p.sizeForTest(); got != 1 {
		t.Fatalf("cache size = %d, want 1", got)
	}
	p.mu.Lock()
	gotHash := p.cache[1].hash
	p.mu.Unlock()
	if gotHash != "h2" {
		t.Fatalf("hash = %q, want h2", gotHash)
	}
}

func TestSweepIdleEvictsStaleKeepsFresh(t *testing.T) {
	p := newTestPool(0) // cap disabled
	stale := &fakeToolSet{name: "stale"}
	fresh := &fakeToolSet{name: "fresh"}

	p.mu.Lock()
	p.cache[1] = &userBundle{hash: "h1", toolsets: []tooliface.ToolSet{stale}, lastUsedAt: time.Now().Add(-2 * mcpIdleTTL)}
	p.cache[2] = &userBundle{hash: "h2", toolsets: []tooliface.ToolSet{fresh}, lastUsedAt: time.Now()}
	p.mu.Unlock()

	p.sweepIdle()

	if !stale.closed.Load() {
		t.Fatal("stale toolset should be Closed")
	}
	if fresh.closed.Load() {
		t.Fatal("fresh toolset should NOT be Closed")
	}
	if p.hasForTest(1) {
		t.Fatal("stale user should be evicted")
	}
	if !p.hasForTest(2) {
		t.Fatal("fresh user should remain")
	}
}

func TestInvalidateClosesAndRemoves(t *testing.T) {
	p := newTestPool(0)
	ts := &fakeToolSet{name: "x"}
	p.insert(7, "h", []tooliface.ToolSet{ts})

	p.Invalidate(7)

	if !ts.closed.Load() {
		t.Fatal("toolset should be Closed on Invalidate")
	}
	if p.hasForTest(7) {
		t.Fatal("user should be removed")
	}
	p.Invalidate(999) // absent user is a no-op, must not panic
}

func TestCloseStopsSweeperAndClosesAll(t *testing.T) {
	p := newTestPool(0)
	a := &fakeToolSet{name: "a"}
	b := &fakeToolSet{name: "b"}
	p.insert(1, "h", []tooliface.ToolSet{a})
	p.insert(2, "h", []tooliface.ToolSet{b})

	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !a.closed.Load() || !b.closed.Load() {
		t.Fatal("all toolsets should be Closed")
	}
	select {
	case <-p.done:
	default:
		t.Fatal("done channel should be closed")
	}
	if err := p.Close(); err != nil { // idempotent
		t.Fatalf("second Close: %v", err)
	}
}

func TestInsertAfterCloseClosesAndDrops(t *testing.T) {
	p := newTestPool(4)
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	late := &fakeToolSet{name: "late"}
	if stored := p.insert(9, "h", []tooliface.ToolSet{late}); stored {
		t.Fatal("insert into a closed pool should report not-stored")
	}
	if !late.closed.Load() {
		t.Fatal("toolset built after Close should be Closed, not leaked")
	}
	if p.hasForTest(9) {
		t.Fatal("closed pool must not cache new bundles")
	}
}

// TestConcurrentAccessIsRaceFree exercises the lock discipline: many goroutines
// insert, sweep, and invalidate at once. Run with -race.
func TestConcurrentAccessIsRaceFree(t *testing.T) {
	p := newTestPool(8)
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			uid := uint(i % 12)
			p.insert(uid, "h", []tooliface.ToolSet{&fakeToolSet{name: "t"}})
			p.sweepIdle()
			p.Invalidate(uid)
		}(i)
	}
	wg.Wait()
	if got := p.sizeForTest(); got > 8 {
		t.Fatalf("cache size = %d, exceeds cap 8", got)
	}
}
