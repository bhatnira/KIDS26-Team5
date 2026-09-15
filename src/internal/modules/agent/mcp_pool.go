package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"antelope/internal/modules/log"
	"antelope/models"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
	tooliface "trpc.group/trpc-go/trpc-agent-go/tool"
	mcptool "trpc.group/trpc-go/trpc-agent-go/tool/mcp"
)

// Eviction tuning for the per-user toolset cache.
const (
	// mcpIdleTTL is how long a user's cached toolsets (and their live MCP
	// connections / stdio subprocesses) survive without being used before the
	// background sweeper closes and evicts them.
	mcpIdleTTL = 30 * time.Minute
	// mcpSweepInterval is how often the sweeper scans for idle bundles.
	mcpSweepInterval = 5 * time.Minute
	// mcpMaxEntries hard-caps the number of cached per-user bundles. Idle TTL
	// alone bounds memory only by how many users are active within one 30-minute
	// window; under a burst of many distinct users the cache (and its live stdio
	// subprocesses) would still grow first and shrink only later. When the cap is
	// reached, inserting a new user's bundle evicts the least-recently-used one.
	// 0 disables the cap.
	mcpMaxEntries = 256
)

// MCPPool resolves and caches the MCP toolsets enabled for a given user.
//
// Each user's enabled MCPConfig rows (plus any global rows with UserID nil)
// are hashed; toolsets are rebuilt only when the hash changes. This keeps
// per-turn MCP connection setup cheap while still respecting settings
// updates immediately on the next turn after a change.
//
// Connection lifecycle: cached toolsets are closed when the pool itself
// shuts down (Close), when the user invalidates their cache after a config
// update (Invalidate), when the hash drifts on a new Resolve call, when a
// background sweeper evicts a bundle idle for mcpIdleTTL, or when the LRU cap
// (mcpMaxEntries) evicts the least-recently-used bundle to make room. The last
// two guards bound memory and open connections/subprocesses on deployments with
// many distinct users, where the cache would otherwise only ever grow.
//
// Locking: p.mu guards only the cache map; it is never held across slow MCP I/O.
// Building toolsets (network/subprocess connect) and closing them happen outside
// the lock, so one slow or hung server can never stall another user's Resolve,
// Invalidate, or the sweeper. Concurrent Resolve calls for the SAME user are
// deduplicated by singleflight so they don't each spawn (and leak) duplicate
// connections.
type MCPPool struct {
	db         *gorm.DB
	maxEntries int // LRU cap on cached bundles; 0 disables.

	mu     sync.Mutex
	cache  map[uint]*userBundle
	closed bool // set by Close; guards against publishing after shutdown.
	// builds collapses concurrent Resolve calls for the same user into one
	// build, keyed by userID.
	builds singleflight.Group

	closeOnce sync.Once
	done      chan struct{}
}

type userBundle struct {
	hash       string
	toolsets   []tooliface.ToolSet
	lastUsedAt time.Time
}

// NewMCPPool returns a ready pool. db must be non-nil; it is used to read
// the user's enabled MCPConfig rows. A background goroutine sweeps idle
// bundles until Close is called.
func NewMCPPool(db *gorm.DB) *MCPPool {
	if db == nil {
		panic("agent.NewMCPPool: db is required")
	}
	p := &MCPPool{
		db:         db,
		maxEntries: mcpMaxEntries,
		cache:      make(map[uint]*userBundle),
		done:       make(chan struct{}),
	}
	go p.sweepLoop()
	return p
}

// Resolve returns the MCP toolsets that should be wired into the agent
// for this user. The same toolsets are returned on every call until the
// underlying config rows change.
//
// Errors from individual MCP servers' initial connect are logged but do
// not fail Resolve — a single broken server should not prevent the rest of
// the agent from working.
func (p *MCPPool) Resolve(ctx context.Context, userID uint) ([]tooliface.ToolSet, error) {
	configs, err := p.loadEnabled(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load mcp configs: %w", err)
	}
	hash := hashConfigs(configs)

	// Fast path: a fresh cache hit only touches the map under the lock.
	p.mu.Lock()
	if b, ok := p.cache[userID]; ok && b.hash == hash {
		b.lastUsedAt = time.Now()
		toolsets := b.toolsets
		p.mu.Unlock()
		return toolsets, nil
	}
	p.mu.Unlock()

	// Slow path: build outside the lock so a slow or hung MCP connect can't
	// block other users. singleflight collapses concurrent builds for the same
	// user so they don't spawn (and leak) duplicate connections; followers share
	// the leader's result.
	//
	// The build runs on context.WithoutCancel(ctx): because followers share the
	// single leader's build, the leader's request being canceled (its caller
	// disconnecting) must not abort the build for everyone else. Detaching keeps
	// the request-scoped values but drops cancellation/deadline; each server's
	// Init stays bounded by its per-connection Timeout (30s default).
	buildCtx := context.WithoutCancel(ctx)
	v, err, _ := p.builds.Do(strconv.FormatUint(uint64(userID), 10), func() (any, error) {
		return p.build(buildCtx, userID, configs, hash)
	})
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	return v.([]tooliface.ToolSet), nil
}

// build (re)creates the user's toolsets and publishes them to the cache. It runs
// under singleflight, so no other build for the same user is in flight. All slow
// MCP I/O — connecting in buildToolsets and closing stale/evicted bundles —
// happens with p.mu released; the lock only guards the map mutations.
func (p *MCPPool) build(ctx context.Context, userID uint, configs []models.MCPConfig, hash string) ([]tooliface.ToolSet, error) { //nolint:unparam // (…, error) kept for forward-compat; current impl always succeeds
	// Detach any stale bundle (hash drift) and close it outside the lock.
	p.mu.Lock()
	var stale []tooliface.ToolSet
	if b, ok := p.cache[userID]; ok {
		stale = b.toolsets
		delete(p.cache, userID)
	}
	p.mu.Unlock()
	closeAll(stale)

	if len(configs) == 0 {
		return nil, nil
	}

	sets := buildToolsets(ctx, configs)
	if !p.insert(userID, hash, sets) {
		// Pool was closed while we were building; insert already closed sets.
		return nil, nil
	}
	return sets, nil
}

// insert publishes a freshly-built bundle, enforcing the LRU cap, and reports
// whether it was stored. Any replaced or evicted bundle's toolsets — and, if the
// pool has been closed, the new bundle's own toolsets — are closed after p.mu is
// released. Safe to call whether or not userID is already cached.
func (p *MCPPool) insert(userID uint, hash string, sets []tooliface.ToolSet) bool {
	var toClose []tooliface.ToolSet
	p.mu.Lock()
	if p.closed {
		// Resolve raced Close (only possible at shutdown). Don't repopulate the
		// drained cache; close what we just built so it can't leak.
		p.mu.Unlock()
		closeAll(sets)
		return false
	}
	if old, ok := p.cache[userID]; ok {
		// Replacing an existing entry — no net growth, so skip cap eviction.
		toClose = append(toClose, old.toolsets...)
		delete(p.cache, userID)
	}
	if p.maxEntries > 0 {
		// Evict least-recently-used bundles until there is room for one more.
		for len(p.cache) >= p.maxEntries {
			victimID, victimSets, ok := p.lruVictimLocked()
			if !ok {
				break
			}
			delete(p.cache, victimID)
			toClose = append(toClose, victimSets...)
			log.L().Info("agent: evicted LRU mcp toolset bundle (cache full)",
				zap.Uint("user_id", victimID), zap.Int("max", p.maxEntries))
		}
	}
	p.cache[userID] = &userBundle{hash: hash, toolsets: sets, lastUsedAt: time.Now()}
	p.mu.Unlock()
	closeAll(toClose)
	return true
}

// buildToolsets connects each config's MCP server. It performs network/subprocess
// I/O and must be called WITHOUT holding p.mu. Errors from an individual server's
// initial connect are logged and that server is skipped — a single broken server
// must not prevent the rest of the agent from working.
func buildToolsets(ctx context.Context, configs []models.MCPConfig) []tooliface.ToolSet {
	sets := make([]tooliface.ToolSet, 0, len(configs))
	for _, cfg := range configs {
		conn, err := toConnectionConfig(cfg)
		if err != nil {
			log.L().Warn("agent: skipping invalid mcp config",
				zap.Uint("id", cfg.ID), zap.String("name", cfg.Name), zap.Error(err))
			continue
		}
		ts := mcptool.NewMCPToolSet(conn, mcptool.WithName(toolsetName(cfg)))
		// Init eagerly so a misconfigured server surfaces an error early.
		if err := ts.Init(ctx); err != nil {
			log.L().Warn("agent: mcp toolset init failed",
				zap.String("name", cfg.Name), zap.Error(err))
			_ = ts.Close()
			continue
		}
		sets = append(sets, ts)
	}
	return sets
}

// lruVictimLocked returns the least-recently-used cached bundle. p.mu must be
// held. The boolean is false only when the cache is empty.
func (p *MCPPool) lruVictimLocked() (uint, []tooliface.ToolSet, bool) {
	var (
		victimID uint
		victim   *userBundle
	)
	for id, b := range p.cache {
		if victim == nil || b.lastUsedAt.Before(victim.lastUsedAt) {
			victimID, victim = id, b
		}
	}
	if victim == nil {
		return 0, nil, false
	}
	return victimID, victim.toolsets, true
}

// Invalidate evicts and closes any cached toolsets for the given user.
// Call after a user adds, edits, or removes an MCP config.
func (p *MCPPool) Invalidate(userID uint) {
	p.mu.Lock()
	var sets []tooliface.ToolSet
	if b, ok := p.cache[userID]; ok {
		sets = b.toolsets
		delete(p.cache, userID)
	}
	p.mu.Unlock()
	closeAll(sets) // outside the lock: a hung Close must not stall the pool
}

// Close stops the sweeper and releases every cached MCP connection. Safe to
// call multiple times.
func (p *MCPPool) Close() error {
	p.closeOnce.Do(func() { close(p.done) })

	p.mu.Lock()
	p.closed = true // any insert racing this shutdown will bail instead of leaking
	old := p.cache
	p.cache = make(map[uint]*userBundle)
	p.mu.Unlock()

	for _, b := range old {
		closeAll(b.toolsets) // outside the lock
	}
	return nil
}

// sweepLoop evicts idle bundles until the pool is closed.
func (p *MCPPool) sweepLoop() {
	ticker := time.NewTicker(mcpSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			p.sweepIdle()
		}
	}
}

// sweepIdle closes and removes every bundle whose toolsets have not been used
// within mcpIdleTTL, freeing the underlying MCP connections / subprocesses. The
// map mutations happen under the lock; the actual Close() calls run after it is
// released so a hung server can't stall Resolve/Invalidate.
func (p *MCPPool) sweepIdle() {
	cutoff := time.Now().Add(-mcpIdleTTL)
	var evicted []tooliface.ToolSet
	p.mu.Lock()
	for userID, b := range p.cache {
		if b.lastUsedAt.Before(cutoff) {
			evicted = append(evicted, b.toolsets...)
			delete(p.cache, userID)
			log.L().Info("agent: evicted idle mcp toolset bundle",
				zap.Uint("user_id", userID))
		}
	}
	p.mu.Unlock()
	closeAll(evicted)
}

// ── helpers ─────────────────────────────────────────────────────────────────

// loadEnabled returns the user's enabled MCP configs combined with any
// globally-shared configs (UserID == NULL).
func (p *MCPPool) loadEnabled(ctx context.Context, userID uint) ([]models.MCPConfig, error) {
	var configs []models.MCPConfig
	err := p.db.WithContext(ctx).
		Where("enabled = ? AND (user_id = ? OR user_id IS NULL)", true, userID).
		Order("id ASC").
		Find(&configs).Error
	if err != nil {
		return nil, err
	}
	return configs, nil
}

func toConnectionConfig(cfg models.MCPConfig) (mcptool.ConnectionConfig, error) {
	transport := strings.ToLower(strings.TrimSpace(cfg.Transport))
	switch transport {
	case "stdio", "sse", "streamable":
	default:
		return mcptool.ConnectionConfig{}, fmt.Errorf("unknown transport %q", cfg.Transport)
	}

	headers, err := unmarshalStringMap(cfg.HeadersJSON)
	if err != nil {
		return mcptool.ConnectionConfig{}, fmt.Errorf("headers_json: %w", err)
	}
	args, err := unmarshalStringSlice(cfg.ArgsJSON)
	if err != nil {
		return mcptool.ConnectionConfig{}, fmt.Errorf("args_json: %w", err)
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return mcptool.ConnectionConfig{
		Transport:   transport,
		ServerURL:   strings.TrimSpace(cfg.ServerURL),
		Headers:     headers,
		Command:     strings.TrimSpace(cfg.Command),
		Args:        args,
		Timeout:     timeout,
		Description: strings.TrimSpace(cfg.Description),
	}, nil
}

func hashConfigs(configs []models.MCPConfig) string {
	if len(configs) == 0 {
		return ""
	}
	h := sha256.New()
	for _, c := range configs {
		fmt.Fprintf(h, "%d|%s|%s|%s|%s|%s|%s|%v|%d|%s\n",
			c.ID, c.Name, c.Transport, c.ServerURL, c.HeadersJSON,
			c.Command, c.ArgsJSON, c.Enabled, c.TimeoutSeconds, c.UpdatedAt.Format(time.RFC3339Nano))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func toolsetName(cfg models.MCPConfig) string {
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return fmt.Sprintf("mcp-%d", cfg.ID)
	}
	return "mcp-" + name
}

func closeAll(sets []tooliface.ToolSet) {
	for _, s := range sets {
		if s == nil {
			continue
		}
		if err := s.Close(); err != nil {
			log.L().Warn("agent: mcp toolset close failed",
				zap.String("name", s.Name()), zap.Error(err))
		}
	}
}

func unmarshalStringMap(s string) (map[string]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	out := make(map[string]string)
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func unmarshalStringSlice(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	return out, nil
}
