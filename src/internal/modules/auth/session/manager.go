package session

import (
	"context"
	"sync"

	"github.com/redis/go-redis/v9"
)

var (
	globalMgr Store
	once      sync.Once
)

// Init initialises the package-level session manager. Must be called before
// Manager() is used. If redisClient is nil a MemoryStore is used as fallback.
func Init(redisClient redis.UniversalClient) {
	once.Do(func() {
		if redisClient != nil {
			globalMgr = NewRedisStore(redisClient)
		} else {
			globalMgr = NewMemoryStore(context.Background())
		}
	})
}

// Manager returns the package-level session store, initialised by Init.
// Panics if Init has not been called.
func Manager() Store {
	if globalMgr == nil {
		panic("session: Manager() called before Init()")
	}
	return globalMgr
}
