package sql

import (
	"sync"

	"gorm.io/gorm"
)

var (
	manager     *Manager
	managerOnce sync.Once
)

// Manager is the SQL connection manager singleton.
// It manages shared GORM DB connections keyed by their DSN string.
// Additional RDBMS drivers (e.g. SQLite, MySQL) can be added by extending
// this package with a new manager_<driver>.go file.
type Manager struct {
	mutex sync.Mutex

	// DBConnections holds open GORM clients keyed by their DSN string.
	DBConnections map[string]*dbHolder
}

// DBClient is a reference-counted handle to a shared *gorm.DB.
// Always call Close when done to release the reference.
// Implements io.Closer.
type DBClient struct {
	DB  *gorm.DB
	dsn string
	mgr *Manager
}

// Close decrements the reference count for the underlying connection.
// When the count reaches zero the connection pool is closed.
func (c *DBClient) Close() error {
	return c.mgr.CloseDBClient(c.dsn)
}

// dbHolder is the internal bookkeeping struct stored inside Manager.
type dbHolder struct {
	*gorm.DB
	dsn   string
	count int64
}

// GetManager returns the singleton Manager, initializing it on the first call.
// Safe for concurrent use.
func GetManager() *Manager {
	managerOnce.Do(func() {
		manager = &Manager{
			DBConnections: make(map[string]*dbHolder),
		}
	})
	return manager
}

// ListConnections returns a snapshot of currently open DSNs and their
// reference counts. Useful for monitoring and debugging.
func (m *Manager) ListConnections() map[string]int64 {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	snapshot := make(map[string]int64, len(m.DBConnections))
	for dsn, h := range m.DBConnections {
		snapshot[dsn] = h.count
	}
	return snapshot
}

// CloseAll forcefully closes every open connection regardless of reference
// count. Intended for graceful shutdown only.
// WARNING: CloseAll forcefully closes every open connection regardless of reference
// count. Intended for graceful shutdown only. Callers must ensure all DBClient
// references have been closed before calling this method to avoid operating on
// closed connections.
func (m *Manager) CloseAll() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var lastErr error
	for dsn, holder := range m.DBConnections {
		delete(m.DBConnections, dsn)
		sqlDB, err := holder.DB.DB()
		if err != nil {
			lastErr = err
			continue
		}
		if err := sqlDB.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}
