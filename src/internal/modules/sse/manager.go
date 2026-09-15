package sse

import (
	"context"
	"sync"
)

// ── Singleton ─────────────────────────────────────────────────────────────────

var (
	globalSSEManager *Manager
	sseOnce          sync.Once
)

// InitManager creates the package-level SSE Manager singleton.
// Safe to call multiple times; only the first call has any effect.
func InitManager() *Manager {
	sseOnce.Do(func() {
		globalSSEManager = NewManager()
	})
	return globalSSEManager
}

// GetManager returns the singleton Manager.
// Panics if InitManager has not been called — this is intentional:
// a missing init is a programming error, not a runtime condition.
func GetManager() *Manager {
	if globalSSEManager == nil {
		panic("sse: global manager not initialised — call InitManager first")
	}
	return globalSSEManager
}

// ── Manager ───────────────────────────────────────────────────────────────────

// Manager tracks active SSE connections and allows graceful shutdown.
type Manager struct {
	connections map[string]context.CancelFunc
	mu          sync.RWMutex
}

// NewManager creates a new connection manager.
func NewManager() *Manager {
	return &Manager{
		connections: make(map[string]context.CancelFunc),
	}
}

// Add registers a new connection with its cancel function.
func (m *Manager) Add(key string, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connections[key] = cancel
}

// Remove removes a connection without cancelling it.
func (m *Manager) Remove(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.connections, key)
}

// Close cancels and removes a single connection.
func (m *Manager) Close(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cancel, ok := m.connections[key]; ok {
		cancel()
	}
	delete(m.connections, key)
}

// CloseAll cancels all connections and clears the map.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, cancel := range m.connections {
		cancel()
	}
	m.connections = make(map[string]context.CancelFunc)
}

// Count returns the number of active connections.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.connections)
}

// Serve registers src with the manager, invokes src.Stream with a cancellable
// context derived from ctx, and removes the registration when Stream returns.
// This is the primary entry-point for running an SSESource through the manager.
func (m *Manager) Serve(ctx context.Context, w *Writer, src SSESource) error {
	id := src.ConnectionID()
	srvCtx, cancel := context.WithCancel(ctx)
	m.Add(id, cancel)
	defer func() {
		cancel()
		m.Remove(id)
	}()
	return src.Stream(srvCtx, w)
}
