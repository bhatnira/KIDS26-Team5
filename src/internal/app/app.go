// Package app is the application root: it initializes all infrastructure
// clients and manages their lifecycle (Shutdown).
//
// Design principles:
//   - App owns only infrastructure: DB, Redis, Nomad, Storage, Mailer, SSE.
//   - App has zero knowledge of HTTP routing, services, or domain models.
//   - All dependencies are injected explicitly; nothing is pulled from globals.
//   - Migration and seeding are supplied by the caller via functional options
//     so this package does not import the models package.
package app

import (
	"context"
	"time"

	"antelope/internal/modules/agent"
	"antelope/internal/modules/email"
	"antelope/internal/modules/llmconfig"
	nixnomad "antelope/internal/modules/nomad"
	"antelope/internal/modules/nosql"
	"antelope/internal/modules/setting"
	nixsql "antelope/internal/modules/sql"
	"antelope/internal/modules/sse"
	"antelope/internal/modules/storage"
	"antelope/internal/modules/validation"
	"antelope/pkg/secretbox"

	nomad "github.com/hashicorp/nomad/api"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// App is the dependency-injection root for an antelope process.
// It owns raw infrastructure clients and exposes them for explicit injection
// into higher layers (services, routers). It never imports domain models or
// HTTP routing packages.
type App struct {
	cfg setting.ServerConfig

	// raw typed clients kept for lifecycle Close()
	dbClient    *nixsql.DBClient
	redisClient *nosql.RedisClient

	// Exposed for explicit injection into services / handlers
	DB        *gorm.DB
	Redis     redis.UniversalClient
	Nomad     *nomad.Client
	Storage   *storage.ClientManager
	Mailer    *email.Mailer
	SSE       *sse.Manager
	LLMConfig *llmconfig.Manager
	Agent     *agent.Manager
	SecretBox *secretbox.Box // shared at-rest encryption box (nil = encryption disabled)
}

// AppDeps exposes only what higher layers (router, server) need from the App.
// Implementations: *App (production), stub structs (tests).
type AppDeps interface {
	GetConfig() setting.ServerConfig
	GetDB() *gorm.DB
	GetRedis() redis.UniversalClient
	GetNomad() *nomad.Client
	GetStorage() *storage.ClientManager
	GetMailer() *email.Mailer
	GetSSE() *sse.Manager
	GetLLMConfig() *llmconfig.Manager
	GetAgent() *agent.Manager
	GetSecretBox() *secretbox.Box
}

// Option is a functional option applied after infrastructure is ready.
// Use it to inject migration and seed logic without creating an import
// dependency between this package and the models / seed packages.
type Option func(*App)

const (
	// domainMigrationLockKey coordinates schema migration across pods.
	domainMigrationLockKey = "antelope:schema-migration"
	// domainMigrationLockTTL must exceed how long AutoMigrate can take.
	domainMigrationLockTTL = 60 * time.Second
	// domainMigrationWait bounds how long a non-winner waits for the winner.
	domainMigrationWait = 120 * time.Second
	domainMigrationPoll = 2 * time.Second
)

// WithMigration returns an Option that runs fn(db) during NewApp.
// fn is typically a closure calling db.AutoMigrate(...).
//
// On a multi-pod rollout every replica would otherwise run AutoMigrate at once,
// and concurrent DDL (CREATE INDEX / ADD COLUMN / FK constraints) can deadlock
// on Postgres ACCESS EXCLUSIVE locks or fatally fail a pod. So migration runs
// under a shared Redis lock (nosql.RunOnce): the winner migrates while others
// wait, then each re-runs the idempotent AutoMigrate (a no-op once the winner
// has applied it) — serialized, never concurrent. If Redis is down it falls
// back to running unguarded, matching the nomad infra-migration posture.
func WithMigration(fn func(db *gorm.DB) error) Option {
	return func(a *App) {
		run := func() error { return fn(a.DB) }
		ctx, cancel := context.WithTimeout(context.Background(), domainMigrationWait+domainMigrationLockTTL)
		defer cancel()

		if err := nosql.RunOnce(ctx, a.Redis, nosql.LockOptions{
			Key:         domainMigrationLockKey,
			TTL:         domainMigrationLockTTL,
			WaitTimeout: domainMigrationWait,
			Poll:        domainMigrationPoll,
		}, run, run); err != nil {
			// Migration failure is always fatal.
			zap.L().Fatal("schema migration failed", zap.Error(err))
		}
		zap.L().Info("schema migration succeeded")
	}
}

// WithSeed returns an Option that runs fn(a) during NewApp.
// fn is typically a closure that seeds the super-user and syncs Redis state.
func WithSeed(fn func(db *gorm.DB, redis redis.UniversalClient, storage *storage.ClientManager)) Option {
	return func(a *App) {
		fn(a.DB, a.Redis, a.Storage)
	}
}

// NewApp initialises the full server infrastructure, applies opts, and
// returns a ready App. Caller should defer app.Shutdown().
//
// Example:
//
//	app := app.NewApp(cfg,
//	    app.WithMigration(models.Migrate),
//	    app.WithSeed(seed.Run),
//	)
func NewApp(cfg setting.ServerConfig, opts ...Option) *App {
	a := &App{cfg: cfg}
	a.initInfrastructure()
	validation.RegisterValidators()
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// NewMonitorApp initialises the minimal subset of infrastructure required by
// the standalone monitor process (no SSE, no storage manager, no mailer).
// Caller should defer app.Shutdown().
func NewMonitorApp(cfg setting.ServerConfig) *App {
	a := &App{cfg: cfg}

	dbClient := nixsql.InitPostgres(cfg.DB, nixsql.PoolConfig{}, cfg.System.GetGinMode())
	a.dbClient = dbClient
	a.DB = dbClient.DB

	redisClient := nosql.InitRedis(cfg.Redis)
	a.redisClient = redisClient
	a.Redis = redisClient.Client

	a.Nomad = nixnomad.InitNomad(cfg.Nomad)
	return a
}

// Shutdown gracefully closes all managed infrastructure in the correct order.
// It is safe to call multiple times.
func (a *App) Shutdown() {
	if a.Agent != nil {
		if err := a.Agent.Close(); err != nil {
			zap.L().Warn("agent manager close failed", zap.Error(err))
		}
	}
	if a.SSE != nil {
		zap.L().Info("closing SSE connections", zap.Int("active", a.SSE.Count()))
		a.SSE.CloseAll()
	}
	if a.dbClient != nil {
		_ = a.dbClient.Close()
	}
	if a.redisClient != nil {
		_ = a.redisClient.Close()
	}
}

// ── Accessors ──────────────────────────────────────────────────────────────
// These exist so callers can treat *App as an AppDeps interface (see deps.go).

func (a *App) GetConfig() setting.ServerConfig    { return a.cfg }
func (a *App) GetDB() *gorm.DB                    { return a.DB }
func (a *App) GetRedis() redis.UniversalClient    { return a.Redis }
func (a *App) GetNomad() *nomad.Client            { return a.Nomad }
func (a *App) GetStorage() *storage.ClientManager { return a.Storage }
func (a *App) GetMailer() *email.Mailer           { return a.Mailer }
func (a *App) GetSSE() *sse.Manager               { return a.SSE }
func (a *App) GetLLMConfig() *llmconfig.Manager   { return a.LLMConfig }
func (a *App) GetAgent() *agent.Manager           { return a.Agent }
func (a *App) GetSecretBox() *secretbox.Box       { return a.SecretBox }

// ── Private ────────────────────────────────────────────────────────────────

func (a *App) initInfrastructure() {
	a.SSE = sse.InitManager()

	dbClient := nixsql.InitPostgres(a.cfg.DB, nixsql.PoolConfig{}, a.cfg.System.GetGinMode())
	a.dbClient = dbClient
	a.DB = dbClient.DB

	redisClient := nosql.InitRedis(a.cfg.Redis)
	a.redisClient = redisClient
	a.Redis = redisClient.Client

	a.Nomad = nixnomad.InitNomad(a.cfg.Nomad)

	// Build the at-rest encryption box once and share it across the secret
	// managers. An empty key disables encryption (configs are persisted as
	// plaintext but still durably in Postgres); a malformed key is fatal.
	secBox := a.buildSecretBox()
	a.SecretBox = secBox

	storage.InitGlobalManager(a.Redis, a.DB, secBox, storage.NewMinioProvider())
	a.Storage = storage.GetGlobalManager()

	a.Mailer = email.InitMailer(a.cfg.Email)

	a.LLMConfig = llmconfig.NewManager(a.Redis, a.DB, secBox)

	agentMgr, err := agent.NewManager(agent.ManagerDeps{
		Cfg:       a.cfg.Agent,
		DBConfig:  a.cfg.DB,
		DB:        a.DB,
		Storage:   a.Storage,
		LLMConfig: a.LLMConfig,
		Box:       secBox,
	})
	if err != nil {
		zap.L().Fatal("agent manager init failed", zap.Error(err))
	}
	a.Agent = agentMgr
}

// buildSecretBox constructs the at-rest encryption box from the configured key.
// An empty key returns nil (encryption disabled — secrets persisted as plaintext
// but still durable); a malformed key is fatal so misconfiguration surfaces at
// startup rather than silently leaving secrets unencrypted.
func (a *App) buildSecretBox() *secretbox.Box {
	key := a.cfg.System.EncryptKey
	if key == "" {
		zap.L().Warn("system.encrypt-key not set; per-user secrets persisted unencrypted at rest")
		return nil
	}
	box, err := secretbox.New(key)
	if err != nil {
		zap.L().Fatal("invalid system.encrypt-key (ANTELOPE_SYSTEM_ENCRYPT_KEY)", zap.Error(err))
	}
	return box
}
