package sql

import (
	"context"
	"fmt"
	"time"

	"antelope/internal/modules/log"
	"antelope/internal/modules/setting"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newGormLogger returns the GORM logger for the given gin mode. In debug mode
// GORM logs every executed SQL statement (and errors), which is invaluable when
// diagnosing problems. In release/test mode it is silenced so failed or slow
// queries don't flood the application logs; callers still receive the error via
// the returned `error`, so nothing is lost operationally.
func newGormLogger(mode string) gormlogger.Interface {
	level := gormlogger.Silent
	if mode == "debug" {
		level = gormlogger.Info
	}
	return gormlogger.Default.LogMode(level)
}

// PoolConfig controls the connection pool behaviour for a single DSN.
// Zero values fall back to sensible defaults.
type PoolConfig struct {
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxLifetime time.Duration
}

func (p PoolConfig) withDefaults() PoolConfig {
	if p.MaxIdleConns == 0 {
		p.MaxIdleConns = 10
	}
	if p.MaxOpenConns == 0 {
		p.MaxOpenConns = 100
	}
	if p.ConnMaxLifetime == 0 {
		p.ConnMaxLifetime = time.Hour
	}
	return p
}

// GetDBClient returns a reference-counted DBClient for the given DSN.
// Call DBClient.Close when done so the underlying pool is released when
// no longer referenced. gormLog controls GORM's query/error logging; pass the
// result of newGormLogger(mode).
func (m *Manager) GetDBClient(dsn string, pool PoolConfig, gormLog gormlogger.Interface) (*DBClient, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if holder, ok := m.DBConnections[dsn]; ok {
		holder.count++
		return &DBClient{DB: holder.DB, dsn: dsn, mgr: m}, nil
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: false,
		// Map driver-specific errors (e.g. Postgres 23505 unique violation) to
		// GORM sentinels so callers can use errors.Is(err, gorm.ErrDuplicatedKey)
		// instead of matching raw SQLSTATE/message strings.
		TranslateError: true,
		Logger:         gormLog,
	})
	if err != nil {
		return nil, fmt.Errorf("sql: failed to open postgres connection: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("sql: failed to retrieve underlying sql.DB: %w", err)
	}

	cfg := pool.withDefaults()
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	holder := &dbHolder{
		DB:    db,
		dsn:   dsn,
		count: 1,
	}
	m.DBConnections[dsn] = holder

	return &DBClient{DB: db, dsn: dsn, mgr: m}, nil
}

// CloseDBClient decrements the reference count for the given DSN and closes
// the underlying connection pool when the count reaches zero.
// Prefer calling DBClient.Close instead of this method directly.
func (m *Manager) CloseDBClient(dsn string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	holder, ok := m.DBConnections[dsn]
	if !ok {
		return nil
	}

	holder.count--
	if holder.count > 0 {
		return nil
	}

	delete(m.DBConnections, dsn)

	sqlDB, err := holder.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Ping verifies that the database behind dsn is still reachable.
// It returns an error if the DSN is not currently managed or the ping fails.
func (m *Manager) Ping(ctx context.Context, dsn string) error {
	m.mutex.Lock()
	holder, ok := m.DBConnections[dsn]
	m.mutex.Unlock()

	if !ok {
		return fmt.Errorf("sql: no active connection for dsn %q", RedactDSN(dsn))
	}

	sqlDB, err := holder.DB.DB()
	if err != nil {
		return fmt.Errorf("sql: failed to retrieve underlying sql.DB: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("sql: ping failed for dsn %q: %w", RedactDSN(dsn), err)
	}
	return nil
}

// InitPostgres opens a PostgreSQL connection from the application config,
// configures the connection pool, and verifies connectivity with a ping.
// It panics on failure so the server does not start with a broken DB connection.
// mode is the gin mode (see setting.SystemConfig.GetGinMode); it controls GORM's
// SQL logging — verbose in debug, silent otherwise.
//
// The returned DBClient must be closed on server shutdown:
//
//	client := InitPostgres(cfg, PoolConfig{}, mode)
//	defer client.Close()
func InitPostgres(cfg setting.DBConfig, pool PoolConfig, mode string) *DBClient {
	dsn := ToPostgresDSN(cfg)

	client, err := GetManager().GetDBClient(dsn, pool, newGormLogger(mode))
	if err != nil {
		panic(fmt.Errorf("sql: InitPostgres failed: %w", err))
	}

	if err := GetManager().Ping(context.Background(), dsn); err != nil {
		_ = client.Close() // close client first
		panic(fmt.Errorf("sql: InitPostgres ping failed: %w", err))
	}

	log.L().Info("connected to PostgreSQL successfully")
	return client
}
