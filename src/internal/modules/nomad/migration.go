package nomad

import (
	"context"
	"fmt"
	"time"

	"antelope/internal/modules/log"
	"antelope/internal/modules/nosql"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const (
	migrationLockKey = "nomad-monitor:migration-lock"
	migrationLockTTL = 60 * time.Second

	// How long a non-leader instance will wait for the leader to finish migration
	// before giving up and returning an error.
	migrationWaitTimeout  = 90 * time.Second
	migrationPollInterval = 2 * time.Second
)

// runMigration ensures the infrastructure tables exist.
//
// Concurrency guarantee
// ─────────────────────
// Migration runs under a shared Redis lock (nosql.RunOnce): only the winner
// executes AutoMigrate; the others wait until the lock is released (migration
// done) and then verify the tables exist via a lightweight check, re-running
// migration only if the winner crashed mid-way. Because AutoMigrate is
// idempotent the worst case is two instances both running it — harmless — but
// the lock makes concurrent DDL extremely unlikely.
func runMigration(ctx context.Context, db *gorm.DB, redisClient redis.UniversalClient) error {
	return nosql.RunOnce(ctx, redisClient, nosql.LockOptions{
		Key:         migrationLockKey,
		TTL:         migrationLockTTL,
		WaitTimeout: migrationWaitTimeout,
		Poll:        migrationPollInterval,
	},
		func() error { return migrate(db) },
		func() error { return verifyTables(db) },
	)
}

// migrate calls GORM AutoMigrate for all infrastructure tables.
func migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&infraCheckPoint{},
		&infraProcessedEvent{},
	); err != nil {
		return fmt.Errorf("nomad infra migration failed: %w", err)
	}
	log.L().Info("nomad infra schema migration complete")
	return nil
}

// verifyTables returns nil when both infrastructure tables exist in the DB.
func verifyTables(db *gorm.DB) error {
	migrator := db.Migrator()
	for _, model := range []any{&infraCheckPoint{}, &infraProcessedEvent{}} {
		if !migrator.HasTable(model) {
			// Table still missing — run migration ourselves as a fallback.
			log.L().Warn("infrastructure table missing after waiting; running migration as fallback")
			return migrate(db)
		}
	}
	return nil
}
