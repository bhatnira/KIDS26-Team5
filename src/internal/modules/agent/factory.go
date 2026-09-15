package agent

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"antelope/internal/modules/agent/skillbundle"
	"antelope/internal/modules/llmconfig"
	"antelope/internal/modules/log"
	"antelope/internal/modules/setting"
	"antelope/internal/modules/storage"
	"antelope/models"
	"antelope/pkg/secretbox"

	"go.uber.org/zap"
	"gorm.io/gorm"
	frameworkagent "trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/artifact"
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// agentName is the framework-internal identifier of the single LLMAgent the
// factory returns. The runner accepts it on every invocation via
// NewRunnerWithAgentFactory; the actual model and tools differ per user.
const agentName = "antelope-agent"

// maxLoadedSkills caps how many skills stay resident in a conversation's state.
// The framework evicts the least-recently-touched beyond this; the model can
// always re-load one. Bundled SKILL.md bodies average ~17 KB, so an uncapped
// session that browses the library would spend its whole context on skills.
const maxLoadedSkills = 4

// FactoryDeps bundles every input the per-request agent assembler needs.
// It is constructed once at startup and reused across all factory calls.
type FactoryDeps struct {
	Cfg       setting.AgentConfig
	DB        *gorm.DB
	LLMConfig *llmconfig.Manager
	Artifacts *PerUserS3Artifact
	Skills    *SkillSync
	MCP       *MCPPool

	// Library is the shared built-in skill library, scanned once at start-up.
	// A nil Library is valid and means the deployment ships no built-in
	// skills — the agent then only sees the user's own uploads.
	Library *skillbundle.Library

	// Storage is the per-user object-storage client manager. The factory uses
	// it to stage chat attachments (s3buf:// / s3url:// inputs) into the
	// sandbox before the agent runs. Required.
	Storage *storage.ClientManager

	// Box is the optional at-rest encryption box (nil = plaintext). Used to
	// decrypt the per-user Daytona API key loaded for the executor.
	Box *secretbox.Box

	// CustomTools are tools that depend on other services (jobsvc,
	// pipelinesvc, storage.ClientManager) and are therefore constructed
	// outside this package. The factory appends them to every agent's
	// tool list. The run_python_inline tool, which only needs the
	// per-request executor, is wired automatically.
	CustomTools []tool.Tool
}

// Factory assembles a fresh LLMAgent for a single chat turn given the
// per-user configuration available in the database, Redis, and disk.
//
// The factory function plugged into runner.NewRunnerWithAgentFactory has
// signature `func(ctx, agent.RunOptions) (agent.Agent, error)`; the runner
// invokes it before constructing the Invocation, so the user identity must
// be carried into ctx by the service layer via WithUserID.
type Factory struct {
	deps FactoryDeps
}

// SetCustomTools replaces the tools the factory appends to every agent.
// Called once during application start-up by routers.go after the
// service-level tool dependencies (jobsvc, storage, etc.) are available.
// Concurrent calls during request serving are not supported.
func (f *Factory) SetCustomTools(tools []tool.Tool) {
	f.deps.CustomTools = tools
}

// NewFactory validates the deps and returns a ready Factory.
func NewFactory(deps FactoryDeps) (*Factory, error) {
	if deps.DB == nil {
		return nil, errors.New("agent factory: db required")
	}
	if deps.LLMConfig == nil {
		return nil, errors.New("agent factory: llm config manager required")
	}
	if deps.Artifacts == nil {
		return nil, errors.New("agent factory: artifact service required")
	}
	if deps.Skills == nil {
		return nil, errors.New("agent factory: skill sync required")
	}
	if deps.MCP == nil {
		return nil, errors.New("agent factory: mcp pool required")
	}
	if deps.Storage == nil {
		return nil, errors.New("agent factory: storage manager required")
	}
	if deps.Cfg.AppName == "" {
		deps.Cfg.AppName = AppName
	}
	return &Factory{deps: deps}, nil
}

// AgentFactoryFunc returns the closure to hand to
// runner.NewRunnerWithAgentFactory. It captures the *Factory so the runner
// can call it for each Run() invocation.
func (f *Factory) AgentFactoryFunc() func(context.Context, frameworkagent.RunOptions) (frameworkagent.Agent, error) {
	return func(ctx context.Context, _ frameworkagent.RunOptions) (frameworkagent.Agent, error) {
		userID, sessionID, ok := RequestFromContext(ctx)
		if !ok {
			return nil, errors.New("agent factory: ctx is missing request identity (caller must use agent.WithRequest)")
		}
		return f.Build(ctx, userID, sessionID)
	}
}

// Build assembles the agent for the given user/session. Exposed as a
// public method so callers (notably tests) can invoke it without going
// through the runner.
func (f *Factory) Build(ctx context.Context, userID uint, sessionID string) (frameworkagent.Agent, error) {
	llmCfg, err := f.deps.LLMConfig.GetDefaultConfig(userID)
	if err != nil {
		return nil, fmt.Errorf("resolve llm config: %w", err)
	}
	if llmCfg == nil {
		return nil, errors.New("no LLM provider is configured for this user (visit Settings → LLM)")
	}

	mdl, err := BuildModel(ctx, llmCfg)
	if err != nil {
		return nil, fmt.Errorf("build model: %w", err)
	}

	exec, err := f.buildExecutor(ctx, userID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("build executor: %w", err)
	}

	// The built-in library was scanned once at start-up and is shared; only the
	// user's own (usually absent) skill directories are resolved per request.
	userDirs, err := f.deps.Skills.Resolve(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("resolve skills: %w", err)
	}
	userRepo, err := skillbundle.UserRepository(userDirs...)
	if err != nil {
		return nil, fmt.Errorf("user skill repo: %w", err)
	}
	repo := skillbundle.NewMulti(f.deps.Library.Repository(), userRepo)

	toolsets, err := f.deps.MCP.Resolve(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("resolve mcp: %w", err)
	}

	// Tools registered with every agent:
	//   - run_python_inline: constructed per-request with the user's
	//     own executor so each turn talks to that user's sandbox.
	//   - skill_search: how the model resolves a built-in skill name, since
	//     the library is too large to enumerate in the prompt. Omitted when
	//     no built-in library is loaded, rather than offered and always empty.
	//   - Caller-provided CustomTools (the 5 Antelope tools wired by
	//     services/agent at app init).
	// The framework's built-in skill_* and workspace_exec tools are
	// registered automatically via WithSkills + WithCodeExecutor.
	customTools := []tool.Tool{
		NewPythonInlineTool(exec),
	}
	if search := NewSkillSearchTool(f.deps.Library.Catalog()); search != nil {
		customTools = append(customTools, search)
	}
	customTools = append(customTools, f.deps.CustomTools...)

	// Carry the user's configured generation params (tokens, temperature,
	// thinking) through to the model. ApplyUserGenerationConfig is the shared
	// source of truth with the connection-test path; it only sets fields the
	// user supplied so unset values fall back to the provider default.
	genCfg := model.GenerationConfig{Stream: true}
	ApplyUserGenerationConfig(&genCfg, llmCfg)

	opts := []llmagent.Option{
		llmagent.WithModel(mdl),
		llmagent.WithDescription("Antelope research agent for bioinformatics and Nextflow pipelines."),
		llmagent.WithInstruction(SystemInstruction),
		llmagent.WithGenerationConfig(genCfg),
		llmagent.WithSkills(repo),
		// A loaded skill's SKILL.md body stays in context for the rest of the
		// conversation, and the bundled ones average ~17 KB (~4k tokens) with a
		// long tail past 50 KB. Cap how many stay resident so a chat that
		// explores several skills cannot crowd out its own history.
		llmagent.WithMaxLoadedSkills(maxLoadedSkills),
		llmagent.WithCodeExecutor(exec),
		// Force skill-mediated execution: the model must call workspace_exec
		// rather than have free-form ```bash blocks scraped from its prose.
		llmagent.WithEnableCodeExecutionResponseProcessor(false),
		llmagent.WithTools(customTools),
	}
	if len(toolsets) > 0 {
		opts = append(opts, llmagent.WithToolSets(toolsets))
	}

	return llmagent.New(agentName, opts...), nil
}

// buildExecutor constructs the per-turn Daytona code executor for this
// user. The executor is LAZY: it is cheap to construct and provisions the
// per-user volume + ephemeral sandbox only on the first code execution, so a
// turn that answers in text (or just reads an inlined attachment) never
// launches a sandbox. It is registered with the in-scope TurnCleanup
// registry so Manager.Run can Close() (delete) it at turn end — a no-op when
// no sandbox was ever created. Filesystem state persists via the volume.
//
// The turn's chat attachments AND the session's saved artifacts are recorded
// for staging into the workspace the first time it is created (i.e. on first
// code execution), so generated code can read every uploaded file at its
// staged inputs path and every prior-turn artifact at its original workspace
// path, regardless of which execution tool it uses. Best-effort — a staging
// failure is logged but does not abort the turn (the model is instructed to
// report missing staged files to the user).
func (f *Factory) buildExecutor(ctx context.Context, userID uint, sessionID string) (codeexecutor.CodeExecutor, error) {
	var ws models.AgentWorkspaceConfig
	err := f.deps.DB.WithContext(ctx).Where("user_id = ?", userID).First(&ws).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("agent workspace is not configured (visit Settings → Agent Workspace)")
		}
		return nil, err
	}

	// Decrypt the Daytona API key before it flows to the executor. Rows written
	// before encryption was enabled (or while no key was configured) carry
	// DaytonaKeyEncrypted=false and are used as-is.
	if ws.DaytonaKeyEncrypted && ws.DaytonaAPIKey != "" {
		if f.deps.Box == nil {
			return nil, errors.New("agent workspace Daytona key is encrypted but no encryption key is configured (set ANTELOPE_SYSTEM_ENCRYPT_KEY)")
		}
		plain, decErr := f.deps.Box.Decrypt(ws.DaytonaAPIKey)
		if decErr != nil {
			return nil, fmt.Errorf("decrypt daytona api key: %w", decErr)
		}
		ws.DaytonaAPIKey = string(plain)
	}

	// The fetcher is primed with any bytes the chat service already pulled
	// for inlining, so small attachments are not fetched from object
	// storage a second time during staging.
	atts := StageAttachmentsFromContext(ctx)
	fetcher := newObjectFetcher(f.deps.Storage, userID, atts)

	// The sandbox is ephemeral per turn, so artifacts saved by earlier turns
	// of this conversation are re-staged at their original workspace paths
	// before any code runs. Best-effort, like attachment staging: a listing
	// failure costs this turn its restored artifacts, not the whole turn.
	// Artifact specs come first so a file the user attached THIS turn wins
	// over a stale same-path artifact.
	arts, listErr := f.deps.Artifacts.ListSessionArtifacts(ctx, artifact.SessionInfo{
		AppName:   f.deps.Cfg.AppName,
		UserID:    strconv.FormatUint(uint64(userID), 10),
		SessionID: sessionID,
	})
	if listErr != nil {
		log.L().Warn("agent: list session artifacts for restore failed (continuing)",
			zap.Uint("user_id", userID),
			zap.String("session_id", sessionID),
			zap.Error(listErr))
	}
	stageSpecs := append(sessionArtifactSpecs(arts), attachmentInputSpecs(atts)...)

	onStageErr := func(stageErr error) {
		log.L().Warn("agent: stage attachments failed (continuing)",
			zap.Uint("user_id", userID),
			zap.String("session_id", sessionID),
			zap.Error(stageErr))
	}

	res, err := BuildSessionExecutor(SessionExecutorParams{
		Cfg:        f.deps.Cfg,
		WS:         ws,
		UserID:     userID,
		SessionID:  sessionID,
		Fetcher:    fetcher,
		StageSpecs: stageSpecs,
		OnStageErr: onStageErr,
	})
	if err != nil {
		return nil, err
	}

	// At turn end: harvest out/ into the artifact store, then close (delete)
	// the sandbox. Both are no-ops if the turn never ran code and thus never
	// provisioned one. The harvest is what makes out/ unconditionally durable
	// — it runs on its own context because the turn context may already be
	// cancelled, and a harvest failure never blocks the sandbox teardown.
	if reg := TurnCleanupFromContext(ctx); reg != nil {
		exec := res.Executor
		appName := f.deps.Cfg.AppName
		artifacts := f.deps.Artifacts
		// The sink (created by Manager.Run) collects what the harvest saves so
		// the runner can surface it to the UI after this closure runs.
		sink := ArtifactSinkFromContext(ctx)
		reg.Register(func() error {
			hctx, cancel := context.WithTimeout(context.Background(), harvestTimeout)
			// warn makes the degraded (non-direct) path observable: it fires
			// per file whose presigned PUT failed, just before the streaming
			// fallback runs.
			warn := func(rel string, directErr error) {
				log.L().Warn("agent: harvest direct upload failed; falling back to streaming",
					zap.Uint("user_id", userID),
					zap.String("session_id", sessionID),
					zap.String("file", rel),
					zap.Error(directErr))
			}
			saved, stats, hErr := harvestOutputs(hctx, artifacts, exec, sink, appName, userID, sessionID, warn)
			cancel()
			sink.Add(saved...)
			if hErr != nil {
				log.L().Warn("agent: harvest outputs failed (continuing)",
					zap.Uint("user_id", userID),
					zap.String("session_id", sessionID),
					zap.String("sandbox_id", exec.SandboxID()),
					zap.Int("saved", stats.Saved()),
					zap.Int("via_url", stats.Direct),
					zap.Int("via_stream", stats.Streamed),
					zap.Error(hErr))
			} else if stats.Saved() > 0 {
				// via_url = presigned PUT straight from the sandbox;
				// via_stream = streaming fallback through the API process.
				log.L().Info("agent: harvested workspace outputs",
					zap.Uint("user_id", userID),
					zap.String("session_id", sessionID),
					zap.Int("saved", stats.Saved()),
					zap.Int("via_url", stats.Direct),
					zap.Int("via_stream", stats.Streamed))
			}
			if cErr := exec.Close(); cErr != nil {
				log.L().Warn("agent: close daytona executor",
					zap.Uint("user_id", userID),
					zap.String("session_id", sessionID),
					zap.String("sandbox_id", exec.SandboxID()),
					zap.Error(cErr))
				return cErr
			}
			return nil
		})
	}

	return res.Executor, nil
}
