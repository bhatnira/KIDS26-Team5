package agent

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"antelope/internal/modules/agent/skillbundle"
	"antelope/internal/modules/llmconfig"
	"antelope/internal/modules/log"
	"antelope/internal/modules/setting"
	"antelope/internal/modules/storage"
	"antelope/pkg/secretbox"

	"go.uber.org/zap"
	"gorm.io/gorm"
	frameworkagent "trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/session"
	"trpc.group/trpc-go/trpc-agent-go/session/postgres"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// harvestPersistTimeout bounds the post-turn write of the harvest event to the
// session store. It runs on a fresh context because the turn context may
// already be cancelled by a client disconnect.
const harvestPersistTimeout = 30 * time.Second

// Manager is the long-lived owner of the agent runner, the session service,
// the per-user artifact service, and the MCP pool.
//
// It is constructed once during application start-up via NewManager and
// shared across all chat handlers. The HTTP-level service layer
// (services/agent) uses Run to stream framework events back to the client.
type Manager struct {
	cfg setting.AgentConfig

	db        *gorm.DB
	runner    runner.Runner
	session   *postgres.Service
	factory   *Factory
	artifacts *PerUserS3Artifact
	mcp       *MCPPool
	skills    *SkillSync
	library   *skillbundle.Library
	box       *secretbox.Box
}

// ManagerDeps bundles the infrastructure the manager needs at construction.
type ManagerDeps struct {
	Cfg       setting.AgentConfig
	DBConfig  setting.DBConfig
	DB        *gorm.DB
	Storage   *storage.ClientManager
	LLMConfig *llmconfig.Manager

	// Box is the optional at-rest encryption box (nil = plaintext). Shared with
	// the storage/LLM secret managers; used to encrypt/decrypt the per-user
	// Daytona API key.
	Box *secretbox.Box

	// CustomTools are passed through to the factory; see FactoryDeps.
	CustomTools []tool.Tool
}

// NewManager wires the session service, artifact service, MCP pool, skill
// sync, factory, and runner. Returns an error on the first wiring failure
// so initialisation failures are reported clearly during app start-up.
func NewManager(deps ManagerDeps) (*Manager, error) {
	if deps.DB == nil {
		return nil, errors.New("agent manager: db required")
	}
	if deps.Storage == nil {
		return nil, errors.New("agent manager: storage manager required")
	}
	if deps.LLMConfig == nil {
		return nil, errors.New("agent manager: llm config manager required")
	}
	if deps.Cfg.AppName == "" {
		deps.Cfg.AppName = AppName
	}

	sess, err := buildSessionService(deps.DBConfig)
	if err != nil {
		return nil, fmt.Errorf("session service: %w", err)
	}

	artifacts := NewPerUserS3Artifact(deps.Storage, deps.DB)
	skills := NewSkillSync(deps.Cfg.Skills, deps.DB)
	mcp := NewMCPPool(deps.DB)

	library, err := openSkillLibrary(deps.Cfg.Skills)
	if err != nil {
		return nil, fmt.Errorf("skill library: %w", err)
	}

	factory, err := NewFactory(FactoryDeps{
		Cfg:         deps.Cfg,
		DB:          deps.DB,
		LLMConfig:   deps.LLMConfig,
		Artifacts:   artifacts,
		Skills:      skills,
		MCP:         mcp,
		Library:     library,
		Storage:     deps.Storage,
		Box:         deps.Box,
		CustomTools: deps.CustomTools,
	})
	if err != nil {
		return nil, fmt.Errorf("agent factory: %w", err)
	}

	r := runner.NewRunnerWithAgentFactory(
		deps.Cfg.AppName,
		agentName,
		factory.AgentFactoryFunc(),
		runner.WithSessionService(sess),
		runner.WithArtifactService(artifacts),
	)

	return &Manager{
		cfg:       deps.Cfg,
		db:        deps.DB,
		runner:    r,
		session:   sess,
		factory:   factory,
		artifacts: artifacts,
		mcp:       mcp,
		skills:    skills,
		library:   library,
		box:       deps.Box,
	}, nil
}

// openSkillLibrary loads the built-in skill library and reports what it found.
//
// A missing library is not fatal: a dev checkout that has not run
// `make skills-bundle` still gets a working agent, just without built-in
// skills. That case is logged as a warning naming the fix, because silently
// running with zero skills looks like a broken agent rather than a missing
// build step.
func openSkillLibrary(cfg setting.AgentSkillsConfig) (*skillbundle.Library, error) {
	lib, err := skillbundle.Open(skillbundle.Options{
		DiskRoot:    cfg.BundleRoot,
		ExtractRoot: cfg.BundleCacheRoot,
	})
	if err != nil {
		return nil, err
	}
	if lib == nil {
		log.L().Warn("agent: no built-in skill library found; the agent will only see " +
			"user-uploaded skills. Run `make skills-bundle` (needs the submodules under " +
			"skills/upstream/), or build with `-tags skills` to embed it")
		return nil, nil
	}
	fields := []zap.Field{
		zap.Int("skills", lib.Len()),
		zap.Int("domains", len(lib.Catalog().Domains())),
		zap.String("root", lib.Root()),
		zap.Bool("embedded", lib.Embedded()),
	}
	if lib.Embedded() {
		// Distinguishes a cold start that paid for the unpack from a restart
		// that reused an already-materialised tree.
		fields = append(fields, zap.Bool("unpacked", lib.Extracted()))
	}
	log.L().Info("agent: built-in skill library loaded", fields...)
	return lib, nil
}

// SecretBox returns the shared at-rest encryption box (nil when no encryption
// key is configured). The service layer uses it to encrypt the per-user
// Daytona API key before persisting it; the factory uses it to decrypt the key
// before handing it to the executor.
func (m *Manager) SecretBox() *secretbox.Box { return m.box }

// KillSandbox releases any Daytona resources tied to a deleted conversation.
// Under the current model there are none to release: code-execution sandboxes
// are ephemeral and deleted at the end of each turn (WithDeleteOnClose), and
// the workspace runs on the sandbox's local disk rather than a persisted
// volume — so a deleted conversation leaves nothing behind. Kept as a no-op
// hook so the call site (DeleteConversation) stays stable if a persistent
// store is reintroduced later.
func (m *Manager) KillSandbox(ctx context.Context, userID uint, sessionID string) error {
	return nil
}

// Run streams agent events for a single chat turn. The caller is
// responsible for translating those events into the wire-level SSE schema
// (see services/agent/stream.go).
//
// userID is the Antelope database id; it is encoded as a string before
// being passed to the framework (which uses string identifiers throughout)
// and also injected into ctx so the AgentFactory can find it.
//
// Run installs a TurnCleanup registry on ctx before invoking the framework
// runner. The factory registers per-turn closers (notably the Daytona
// ephemeral sandbox) on that registry; Run drains it once the upstream
// event channel closes so per-turn resources are released without leaking
// even on early errors or client disconnect.
func (m *Manager) Run(
	ctx context.Context,
	userID uint,
	sessionID string,
	message model.Message,
	attachments []AttachmentRef,
) (<-chan *event.Event, error) {
	if userID == 0 {
		return nil, errors.New("agent run: user id is required")
	}
	if sessionID == "" {
		return nil, errors.New("agent run: session id is required")
	}

	ctx = WithRequest(ctx, userID, sessionID)
	ctx = WithStageAttachments(ctx, attachments)
	// The sink collects what the end-of-turn harvest saves; the factory's
	// harvest closure fills it and this goroutine emits it once cleanup runs.
	sink := &ArtifactSink{}
	ctx = WithArtifactSink(ctx, sink)
	ctx, cleanup := WithTurnCleanup(ctx)

	logCleanupErr := func(err error) {
		log.L().Warn("agent turn cleanup error",
			zap.Uint("user_id", userID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
	// Drain reports its first error as a return value and the rest via the
	// callback; log the returned one too so no cleanup failure is dropped.
	drainCleanup := func() {
		if err := cleanup.Drain(logCleanupErr); err != nil {
			logCleanupErr(err)
		}
	}

	upstream, err := m.runner.Run(
		ctx, strconv.FormatUint(uint64(userID), 10), sessionID, message,
		// Replace the framework's default "Available skills" block. Its default
		// prints one name+description line per visible skill, which for the
		// built-in library is ~100k tokens on every request; this renders domain
		// counts plus the user's own skills and points the model at
		// skill_search. See skillbundle.RenderOverview.
		frameworkagent.WithAvailableSkillsRenderer(m.renderSkillOverview),
	)
	if err != nil {
		// runner.Run returning an error means factory.Build never ran to
		// completion (or it ran and registered a closer that we still
		// need to drain). Either way, draining is safe — it's a no-op
		// when nothing was registered.
		drainCleanup()
		return nil, err
	}

	out := make(chan *event.Event)
	// Install the live progress emitter the harvest uses to stream pending /
	// per-file / settled events. It only fires during the harvest (after the
	// loop below drains upstream), so it never races the forwarding sends.
	sink.SetEmitter(func(ev *event.Event) {
		if ev == nil {
			return
		}
		select {
		case out <- ev:
		case <-ctx.Done():
		}
	})
	go func() {
		defer close(out)
		for ev := range upstream {
			select {
			case out <- ev:
			case <-ctx.Done():
				// Drain remaining upstream events so the framework's
				// producer goroutine can exit. We've already lost
				// interest in them, but still run cleanup (which performs
				// the output harvest) so generated files are not lost, and
				// still persist the consolidated harvest event so a later
				// reload shows the result cards. Live progress emission is a
				// no-op now — the emitter's send loses the race to ctx.Done.
				for range upstream {
				}
				drainCleanup()
				m.finishHarvest(userID, sessionID, sink)
				return
			}
		}
		// Normal completion: run per-turn cleanup. The harvest streams its
		// progress (pending → per-file cards → settled) live through the sink
		// emitter as it uploads; here we only persist the consolidated event
		// so the cards survive a conversation reload. That event never
		// re-enters the framework pipeline; its structured payload rides in
		// StateDelta so the model only ever sees the short summary.
		drainCleanup()
		m.finishHarvest(userID, sessionID, sink)
	}()
	return out, nil
}

// renderSkillOverview renders the request-scoped "Available skills" section.
// The framework calls it with the skills visible to this request, which is the
// shared built-in library plus whatever the user has uploaded.
func (m *Manager) renderSkillOverview(
	_ context.Context,
	req frameworkagent.AvailableSkillsRenderRequest,
) string {
	return skillbundle.RenderOverview(m.library.Catalog(), req.Summaries, SkillSearchToolName)
}

// SkillLibrary exposes the built-in library so the service layer can list and
// read skills without going through the agent runner. Nil when the deployment
// ships none.
func (m *Manager) SkillLibrary() *skillbundle.Library { return m.library }

// finishHarvest persists the consolidated harvest event (all files saved this
// turn) so the result cards are reconstructed on conversation reload. The live
// cards were already emitted progressively by the harvest itself. A no-op when
// nothing was harvested.
func (m *Manager) finishHarvest(userID uint, sessionID string, sink *ArtifactSink) {
	harvested := sink.Drain()
	if len(harvested) == 0 {
		return
	}
	ev := newHarvestEvent(harvested)
	if ev == nil {
		return
	}
	m.persistHarvestEvent(userID, sessionID, ev)
}

// persistHarvestEvent appends the harvest event to the session store so the
// result cards are reconstructed on conversation reload (see
// services/agent.serializeEvents). Best-effort: a failure costs the reloaded
// transcript its cards, never the turn. Runs on a fresh, bounded context.
func (m *Manager) persistHarvestEvent(userID uint, sessionID string, ev *event.Event) {
	if ev == nil || m.session == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), harvestPersistTimeout)
	defer cancel()

	key := session.Key{
		AppName:   m.cfg.AppName,
		UserID:    strconv.FormatUint(uint64(userID), 10),
		SessionID: sessionID,
	}
	sess, err := m.session.GetSession(ctx, key)
	if err != nil || sess == nil {
		if err != nil {
			log.L().Warn("agent: load session for harvest persist failed (continuing)",
				zap.Uint("user_id", userID),
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
		return
	}
	if err := m.session.AppendEvent(ctx, sess, ev); err != nil {
		log.L().Warn("agent: persist harvest event failed (continuing)",
			zap.Uint("user_id", userID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// SetCustomTools wires the service-built tools (the 5 Antelope tools) into
// the agent factory. Called once during start-up after jobsvc/storage/etc
// have been constructed.
func (m *Manager) SetCustomTools(tools []tool.Tool) {
	m.factory.SetCustomTools(tools)
}

// Artifacts exposes the artifact service so the HTTP layer can serve
// list/download requests without going through the runner.
func (m *Manager) Artifacts() *PerUserS3Artifact { return m.artifacts }

// Session exposes the framework session service so services/agent can
// implement conversation CRUD on top of it.
func (m *Manager) Session() *postgres.Service { return m.session }

// MCP returns the MCP pool, used by the settings handler to invalidate the
// cache after a user changes their MCP configuration.
func (m *Manager) MCP() *MCPPool { return m.mcp }

// Close shuts the runner, MCP pool, and session service down. Safe to call
// multiple times.
func (m *Manager) Close() error {
	var firstErr error
	if m.runner != nil {
		if err := m.runner.Close(); err != nil {
			firstErr = err
		}
	}
	if m.mcp != nil {
		if err := m.mcp.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	// The framework session service has its own internal cleanup goroutine
	// that exits when the pgx pool is closed during process shutdown.
	return firstErr
}

// buildSessionService constructs the framework's postgres-backed session
// store. It opens its own pgx pool — sharing the GORM connection would
// require a custom storage/postgres client builder, which is not worth the
// complexity for v1.
func buildSessionService(db setting.DBConfig) (*postgres.Service, error) {
	opts := []postgres.ServiceOpt{
		postgres.WithHost(db.Host),
		postgres.WithPort(db.Port),
		postgres.WithUser(db.Username),
		postgres.WithPassword(db.Password),
		postgres.WithDatabase(db.DB),
	}
	if mode := db.SSLMode; mode != "" {
		opts = append(opts, postgres.WithSSLMode(mode))
	}
	return postgres.NewService(opts...)
}
