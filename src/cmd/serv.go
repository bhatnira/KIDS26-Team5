package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"antelope/internal/modules/agent"
	"antelope/internal/modules/email"
	"antelope/internal/modules/llmconfig"
	nixnomad "antelope/internal/modules/nomad"
	"antelope/internal/modules/setting"
	"antelope/internal/modules/sse"
	"antelope/internal/modules/storage"
	"antelope/pkg/secretbox"
	"antelope/routers"
	monitorsvc "antelope/services/monitor"

	"github.com/gin-gonic/gin"
	nomad "github.com/hashicorp/nomad/api"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Package server owns the HTTP server lifecycle: it wires App infrastructure
// to the RouterManager, starts the HTTP listener and the Nomad event monitor,
// and handles graceful shutdown on SIGINT/SIGTERM.
//
// Responsibility split:
//
//	app.App      – infrastructure (DB, Redis, Nomad, Storage, Mailer, SSE)
//	routers.RouterManager – service wiring + route registration
//	server.Server – HTTP server + monitor + process signal handling

// AppDeps is the minimal interface Server depends on.
// Identical surface to routers.AppDeps so both packages stay decoupled from
// *app.App. Duplicating the interface is intentional – it keeps each package's
// dependency surface explicit and independently mockable in tests.
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
	Shutdown()
}

// Server owns the HTTP server lifecycle and the Nomad event monitor.
// It is constructed once in main and driven by Run() or RunMonitor().
type Server struct {
	deps   AppDeps
	cfg    setting.ServerConfig
	router *routers.RouterManager

	// engine and httpSrv are set when Run/RunMonitor is called.
	engine  *gin.Engine
	httpSrv *http.Server
}

// NewServer creates a Server. It builds the RouterManager (which eagerly constructs
// all services) so construction failures surface before any listener starts.
func NewServer(deps AppDeps) *Server {
	return &Server{
		deps:   deps,
		cfg:    deps.GetConfig(),
		router: routers.New(deps),
	}
}

// Engine returns the main API gin.Engine (lazily built on first call).
// Exposed for integration tests that want to call httptest against the engine
// without starting a real listener.
func (s *Server) Engine() *gin.Engine {
	if s.engine == nil {
		s.engine = s.router.Engine()
	}
	return s.engine
}

// RunWeb starts the main HTTP API server and the Nomad event monitor in the same
// process, then blocks until SIGINT/SIGTERM.
//
// Shutdown order on signal:
//  1. Stop Nomad event monitor (releases HA leader lock)
//  2. Close all SSE connections
//  3. Gracefully shut down HTTP server (5 s timeout)
//  4. Close DB / Redis (deferred via deps.Shutdown)
func (s *Server) RunWeb() {
	addr := fmt.Sprintf(":%d", s.cfg.System.Port)
	s.httpSrv = &http.Server{
		Addr:           addr,
		Handler:        s.Engine(),
		ReadTimeout:    10 * time.Minute,
		WriteTimeout:   10 * time.Minute,
		MaxHeaderBytes: 1 << 20,
	}

	go func() {
		zap.L().Info("HTTP server starting", zap.String("addr", addr))
		if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			zap.L().Error("HTTP server error", zap.Error(err))
			os.Exit(1)
		}
	}()

	monitor := s.mustCreateMonitor()
	go func() {
		zap.L().Info("Nomad event monitor starting")
		if err := monitor.StartWithHA(); err != nil {
			zap.L().Error("event monitor error", zap.Error(err))
			os.Exit(1)
		}
	}()

	// Resolve records orphaned by a pod that died mid-dispatch (Redis-locked so
	// only one instance sweeps). Off the hot path; safe to run after listen.
	// A startup pass handles the rollout; a periodic sweep then covers crashes on
	// a long-lived fleet. reconcileCtx is cancelled on shutdown to stop the loop.
	reconcileCtx, cancelReconcile := context.WithCancel(context.Background())
	defer cancelReconcile()
	go s.router.RunStartupReconciliation()
	go s.router.RunPeriodicReconciliation(reconcileCtx)

	s.waitForSignal()

	cancelReconcile() // stop the periodic reconciler promptly
	monitor.Shutdown()
	zap.L().Info("Nomad event monitor stopped")

	if sseM := s.deps.GetSSE(); sseM != nil {
		zap.L().Info("closing SSE connections", zap.Int("active", sseM.Count()))
		sseM.CloseAll()
		zap.L().Info("SSE connections closed")
	}

	time.Sleep(1 * time.Second) // let in-flight SSE consumers drain

	s.shutdownHTTP()
	zap.L().Info("server shutdown complete")
}

// RunMonitor starts only the standalone Nomad event monitor with a minimal
// HTTP health endpoint on port+1.
func (s *Server) RunMonitor() {
	monitorEngine := s.router.MonitorEngine()
	addr := fmt.Sprintf(":%d", s.cfg.System.Port+1)
	srv := &http.Server{
		Addr:           addr,
		Handler:        monitorEngine,
		ReadTimeout:    10 * time.Minute,
		WriteTimeout:   10 * time.Minute,
		MaxHeaderBytes: 1 << 20,
	}

	monitor := s.mustCreateMonitor()
	go func() {
		if err := monitor.StartWithHA(); err != nil {
			zap.L().Error("monitor start failed", zap.Error(err))
			os.Exit(1)
		}
	}()

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			zap.L().Error("monitor HTTP server start failed", zap.Error(err))
			os.Exit(1)
		}
	}()

	s.waitForSignal()

	monitor.Shutdown()
	zap.L().Info("shutting down monitor server")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		zap.L().Error("monitor server shutdown error", zap.Error(err))
	}
	zap.L().Info("monitor server shutdown complete")
}

// ── private helpers ────────────────────────────────────────────────────────

func (s *Server) mustCreateMonitor() *nixnomad.EventMonitor {
	handler := monitorsvc.NewEventHandler(s.deps.GetDB(), s.deps.GetRedis())
	monitor, err := nixnomad.NewEventMonitor(
		s.deps.GetNomad(),
		s.deps.GetDB(),
		s.deps.GetRedis(),
		handler,
	)
	if err != nil {
		zap.L().Fatal("failed to create event monitor", zap.Error(err))
	}
	return monitor
}

func (s *Server) waitForSignal() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	zap.L().Info("shutdown signal received")
}

func (s *Server) shutdownHTTP() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.httpSrv.Shutdown(ctx); err != nil {
		zap.L().Error("HTTP server shutdown error", zap.Error(err))
	}
}
