package nosql

import (
	"sync"
)

var (
	manager     *Manager
	managerOnce sync.Once
)

// Manager is the nosql connection manager singleton.
// It manages shared connections to nosql backends (currently Redis).
// Additional backends (e.g. LevelDB) can be added by extending this struct.
type Manager struct {
	mutex sync.Mutex

	// RedisConnections holds open Redis clients keyed by all their alias strings.
	// All alias keys (original connection string, canonical URI, clientname) point
	// to the same *redisClientHolder so any of them can be used to look up or
	// release the connection.
	RedisConnections map[string]*redisClientHolder
}

// redisClientHolder is the internal bookkeeping struct stored inside Manager.
// It is not exported; callers interact with RedisClient instead.
type redisClientHolder struct {
	client UniversalRedisClient
	// canonical is the normalized URI for this connection. Stored explicitly
	// so we never have to rely on aliases index ordering.
	canonical string
	// aliases stores every key (original string, canonical URI, clientname)
	// that maps to this holder so they can all be removed atomically on close.
	aliases []string
	count   int64
}

// GetManager returns the singleton Manager, initializing it on the first call.
// Safe for concurrent use.
func GetManager() *Manager {
	managerOnce.Do(func() {
		manager = &Manager{
			RedisConnections: make(map[string]*redisClientHolder),
		}
	})
	return manager
}

// ListConnections returns a snapshot of canonical URIs and their reference
// counts for all currently open connections. Useful for monitoring/debugging.
func (m *Manager) ListConnections() map[string]int64 {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	seen := make(map[*redisClientHolder]bool)
	snapshot := make(map[string]int64)
	for _, h := range m.RedisConnections {
		if seen[h] {
			continue
		}
		seen[h] = true
		// Use the explicitly stored canonical field instead of aliases index.
		snapshot[h.canonical] = h.count
	}
	return snapshot
}

// CloseAll forcefully closes every open Redis connection regardless of
// reference count. Intended for graceful shutdown only.
func (m *Manager) CloseAll() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Collect unique holders first, then clear the map atomically, then close.
	// This avoids the confusing pattern of deleting keys while ranging over the
	// map and relying on `seen` to prevent double-close.
	seen := make(map[*redisClientHolder]bool)
	for _, holder := range m.RedisConnections {
		seen[holder] = true
	}
	// Reset the map in one shot so no alias keys are left dangling.
	m.RedisConnections = make(map[string]*redisClientHolder)

	var lastErr error
	for holder := range seen {
		if err := holder.client.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}
