// Package daytona provides a CodeExecutor implementation backed by a
// persistent Daytona sandbox. Unlike ephemeral executors (E2B, container),
// this executor uses a "persistent workspace model": the sandbox and its
// filesystem survive across agent invocations. CreateWorkspace is idempotent —
// it ensures the standard layout exists and always returns the same physical
// path, so installed dependencies, staged inputs, and intermediate files are
// available across conversation turns without re-upload.
//
// # Typical usage (data-analysis agent)
//
//	exec, err := daytona.New(
//	    daytona.WithSandboxName("user-123-session-abc"),  // reuse across turns
//	    daytona.WithAutoStopInterval(30),                  // stop after 30 min idle
//	)
//	// wrap with init hook to install deps once
//	exec, err = codeexecutor.NewWorkspaceInitExecutor(exec,
//	    codeexecutor.NewWorkspaceInitHook(codeexecutor.WorkspaceInitSpec{
//	        Commands: []codeexecutor.WorkspaceInitCommand{
//	            {Key: "pip-install", Cmd: "pip", Args: []string{"install", "-q", "pandas", "matplotlib"}},
//	        },
//	    }),
//	)
package daytona

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	dtclient "github.com/daytonaio/daytona/libs/sdk-go/pkg/daytona"
	dtoptions "github.com/daytonaio/daytona/libs/sdk-go/pkg/options"
	dttypes "github.com/daytonaio/daytona/libs/sdk-go/pkg/types"
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	"trpc.group/trpc-go/trpc-agent-go/log"
)

// Compile-time interface checks.
var (
	_ codeexecutor.CodeExecutor   = (*CodeExecutor)(nil)
	_ codeexecutor.EngineProvider = (*CodeExecutor)(nil)
)

// ─── Options ─────────────────────────────────────────────────────────────────

// Option configures a CodeExecutor.
type Option func(*CodeExecutor)

// WithAPIKey sets the Daytona API key. Falls back to DAYTONA_API_KEY env var.
func WithAPIKey(key string) Option {
	return func(c *CodeExecutor) { c.apiKey = key }
}

// WithAPIURL overrides the Daytona API base URL (e.g. "https://app.daytona.io/api").
// Falls back to DAYTONA_API_URL env var.
func WithAPIURL(url string) Option {
	return func(c *CodeExecutor) { c.apiURL = url }
}

// WithSandboxID connects to an existing sandbox by ID (does not create one).
// The sandbox is NOT stopped on Close — it persists for future sessions.
func WithSandboxID(id string) Option {
	return func(c *CodeExecutor) { c.sandboxID = id }
}

// WithSandboxName connects to an existing sandbox by name, or creates one with
// that name if it does not exist. Subsequent calls with the same name reuse the
// sandbox and its workspace filesystem.
func WithSandboxName(name string) Option {
	return func(c *CodeExecutor) { c.sandboxName = name }
}

// WithSnapshot sets the snapshot used when creating a new sandbox.
func WithSnapshot(snapshot string) Option {
	return func(c *CodeExecutor) { c.snapshot = snapshot }
}

// WithEnvVars sets environment variables injected into the sandbox at creation.
func WithEnvVars(vars map[string]string) Option {
	return func(c *CodeExecutor) { c.envVars = vars }
}

// WithLabels attaches labels to the sandbox (useful for listing/filtering).
func WithLabels(labels map[string]string) Option {
	return func(c *CodeExecutor) { c.labels = labels }
}

// WithWorkspacePath overrides the root path inside the sandbox where the agent
// workspace is created. Default: /home/user/trpc_agent_workspace.
func WithWorkspacePath(p string) Option {
	return func(c *CodeExecutor) { c.workspacePath = p }
}

// WithSandboxTimeout sets the maximum time to wait for sandbox start.
func WithSandboxTimeout(t time.Duration) Option {
	return func(c *CodeExecutor) { c.sandboxTimeout = t }
}

// WithAutoStopInterval sets the sandbox auto-stop interval in minutes.
// nil disables auto-stop; 0 means immediate stop after creation.
// The value is embedded in the create params and also applied via
// SetAutoArchiveInterval when connecting to an existing sandbox.
func WithAutoStopInterval(minutes int) Option {
	return func(c *CodeExecutor) { c.autoStopMinutes = &minutes }
}

// WithVolumeMount attaches a persistent Daytona volume to the sandbox at the
// executor's workspace path. When subPath is non-empty, only that subtree of
// the volume is mounted (and is the visible root inside the sandbox).
//
// Typical usage is one volume per user with subPath=session-{id} so the
// workspace filesystem survives across short-lived per-turn sandboxes while
// still being isolated per conversation.
//
// This option only applies when the executor creates a new sandbox; mounting
// onto a pre-existing sandbox is a Daytona platform restriction.
func WithVolumeMount(volumeID, subPath string) Option {
	return func(c *CodeExecutor) {
		if volumeID == "" {
			return
		}
		mount := volumeMount{VolumeID: volumeID, SubPath: subPath}
		c.volumes = append(c.volumes, mount)
	}
}

// WithDeleteOnClose makes Close() delete the sandbox instead of stopping it.
// Intended for the ephemeral per-turn lifecycle: the sandbox is disposable
// because durable state lives on an attached volume. Without this option the
// historical behaviour applies (stop only when this executor created the
// sandbox).
func WithDeleteOnClose() Option {
	return func(c *CodeExecutor) { c.deleteOnClose = true }
}

// WithObjectFetcher injects the capability used to resolve s3buf:// and
// s3url:// workspace inputs (user-uploaded chat attachments). It is bound to
// one user's object-storage client by the caller; daytona itself stays
// decoupled from the storage module. nil disables s3 input staging.
func WithObjectFetcher(f ObjectFetcher) Option {
	return func(c *CodeExecutor) { c.fetcher = f }
}

// WithInsecureTLSTransfers makes the in-sandbox curl/wget transfers (presigned
// PUT harvest, presigned GET staging) skip TLS certificate verification.
//
// DEV ONLY — TEMPORARY. Lets a local self-signed storage endpoint use the
// direct streaming path. Never enable in production. See
// setting.AgentDaytonaConfig.InsecureTLSTransfers.
func WithInsecureTLSTransfers() Option {
	return func(c *CodeExecutor) { c.insecureTLS = true }
}

// WithLazyStart defers sandbox provisioning until the first code-execution
// operation. Without it, New creates the sandbox eagerly (the historical
// behaviour, still used by transient/maintenance executors). The chat path
// uses lazy start so a turn that only answers in text — or only reads an
// inlined attachment — never launches a sandbox.
func WithLazyStart() Option {
	return func(c *CodeExecutor) { c.lazyStart = true }
}

// WithLazyVolume records a per-user volume to ensure and mount when the
// sandbox is lazily created. Unlike WithVolumeMount (which needs an
// already-resolved volume ID), this resolves the ID via EnsureVolume only on
// first use — so a turn that never runs code makes no volume API calls.
// Requires WithLazyStart.
func WithLazyVolume(volumeName, subPath string) Option {
	return func(c *CodeExecutor) {
		c.lazyVolumeName = volumeName
		c.lazyVolumeSubPath = subPath
	}
}

// WithStageInputs records inputs to stage into the workspace the first time
// it is created (on first code execution). Empty/nil is a no-op.
func WithStageInputs(specs []codeexecutor.InputSpec) Option {
	return func(c *CodeExecutor) { c.stageSpecs = specs }
}

// WithStageErrorHandler sets a callback invoked when first-use input staging
// fails. Staging is best-effort and never blocks code execution; this just
// lets the caller log the failure with its own request context. When unset,
// failures are logged at warn level by this package.
func WithStageErrorHandler(h func(error)) Option {
	return func(c *CodeExecutor) { c.stageErrHandler = h }
}

// ObjectFetcher reads objects from the user's storage bucket so the workspace
// runtime can stage chat attachments into the sandbox. Two transfer paths are
// exposed so the caller can pick per file size:
//
//   - GetObject buffers the whole object in the API process, then the runtime
//     uploads it via the Daytona SDK. Cheap for small files; memory-heavy for
//     large ones.
//   - PresignGet returns a time-limited URL the sandbox downloads itself
//     (curl/wget), streaming object store → volume without touching the API
//     process. Scales to multi-GB files but needs sandbox egress to the
//     storage endpoint.
type ObjectFetcher interface {
	GetObject(ctx context.Context, bucket, key string) ([]byte, error)
	PresignGet(ctx context.Context, bucket, key string, ttl time.Duration) (string, error)
}

// ─── CodeExecutor ────────────────────────────────────────────────────────────

// volumeMount describes a Daytona volume to attach at the executor's
// workspace path. SubPath is optional (empty mounts the whole volume).
type volumeMount struct {
	VolumeID string
	SubPath  string
}

// CodeExecutor executes code and manages a persistent workspace in a
// Daytona sandbox.
type CodeExecutor struct {
	mu sync.Mutex

	// Connection.
	apiKey      string
	apiURL      string
	sandboxID   string
	sandboxName string
	snapshot    string
	envVars     map[string]string
	labels      map[string]string
	volumes     []volumeMount

	// Timing.
	sandboxTimeout  time.Duration
	autoStopMinutes *int

	// Workspace root path inside the sandbox.
	workspacePath string

	// Lifecycle.
	deleteOnClose bool // true → Close() deletes the sandbox

	// fetcher resolves s3buf:// / s3url:// workspace inputs. nil disables
	// attachment staging (the resolver returns a clear error).
	fetcher ObjectFetcher

	// insecureTLS skips TLS verification on in-sandbox curl/wget transfers.
	// DEV ONLY — see WithInsecureTLSTransfers.
	insecureTLS bool

	// Lazy provisioning. When lazyStart is set, New does NOT create the
	// sandbox; it is provisioned on the first code-execution operation
	// (ensureSandbox). lazyVolumeName/SubPath are resolved to a volume mount
	// at that same point, so a turn that never runs code makes no Daytona
	// volume or sandbox API calls at all.
	lazyStart         bool
	lazyVolumeName    string
	lazyVolumeSubPath string

	// stageSpecs are staged into the workspace the first time it is created
	// (i.e. on first code execution). stageOnce guards that one-shot run;
	// stageErrHandler, when set, receives a staging failure instead of the
	// default warn log (lets the caller log with request context).
	stageSpecs      []codeexecutor.InputSpec
	stageOnce       sync.Once
	stageErrHandler func(error)

	// Live state. With lazy start, sbx stays nil until ensureSandbox runs.
	client *dtclient.Client
	sbx    *dtclient.Sandbox
	rt     *workspaceRuntime
	owned  bool // true when this executor created the sandbox
}

const (
	defaultWorkspacePath  = "/home/user/trpc_agent_workspace"
	defaultSandboxTimeout = 2 * time.Minute

	// defaultVolumeReadyTimeout caps how long a lazily-resolved volume may
	// take to become ready before the first sandbox provisioning.
	defaultVolumeReadyTimeout = 2 * time.Minute
)

// New creates or connects to a Daytona sandbox and returns a CodeExecutor.
// See package docs for the persistent workspace model.
func New(opts ...Option) (*CodeExecutor, error) {
	return NewWithContext(context.Background(), opts...)
}

// NewWithContext is like New but accepts a context used for sandbox setup.
func NewWithContext(ctx context.Context, opts ...Option) (*CodeExecutor, error) {
	c := &CodeExecutor{
		sandboxTimeout: defaultSandboxTimeout,
		workspacePath:  defaultWorkspacePath,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	if c.workspacePath == "" {
		c.workspacePath = defaultWorkspacePath
	}

	cfg := &dttypes.DaytonaConfig{}
	if c.apiKey != "" {
		cfg.APIKey = c.apiKey
	}
	if c.apiURL != "" {
		cfg.APIUrl = c.apiURL
	}

	client, err := dtclient.NewClientWithConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("daytona: create client: %w", err)
	}
	c.client = client
	c.rt = newWorkspaceRuntime(c)

	// Lazy start: defer sandbox (and lazy-volume) creation to first use. The
	// executor is otherwise fully usable — ensureSandbox runs on the first
	// code-execution operation.
	if c.lazyStart {
		log.Debugf("daytona executor created (lazy); sandbox deferred until first code execution")
		return c, nil
	}

	sbx, owned, err := c.acquireSandbox(ctx, client)
	if err != nil {
		// Close the client so we don't leak its HTTP pool when sandbox
		// acquisition fails. Best-effort.
		_ = client.Close(context.Background())
		return nil, err
	}
	c.sbx = sbx
	c.owned = owned

	log.Debugf("daytona sandbox ready: id=%s name=%s owned=%v workspace=%s",
		sbx.ID, sbx.Name, owned, c.workspacePath)
	return c, nil
}

// ensureSandbox lazily provisions the sandbox (and resolves/mounts the lazy
// volume) on first use. It is a no-op once the sandbox exists, and is safe
// for concurrent callers. Every workspaceRuntime entry point and ExecuteCode
// call it before touching the sandbox, so a turn that never runs code never
// reaches here and therefore never creates a sandbox.
func (c *CodeExecutor) ensureSandbox(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sbx != nil {
		return nil
	}
	if c.client == nil {
		return errors.New("daytona: executor is closed")
	}

	// Resolve + mount the per-user volume lazily so a turn that never runs
	// code makes no volume API calls either.
	if c.lazyVolumeName != "" {
		volumeID, err := EnsureVolume(ctx, c.apiKey, c.apiURL, c.lazyVolumeName, defaultVolumeReadyTimeout)
		if err != nil {
			return fmt.Errorf("daytona: ensure volume %q: %w", c.lazyVolumeName, err)
		}
		c.volumes = append(c.volumes, volumeMount{VolumeID: volumeID, SubPath: c.lazyVolumeSubPath})
		c.lazyVolumeName = "" // mounted now; don't re-resolve
	}

	sbx, owned, err := c.acquireSandbox(ctx, c.client)
	if err != nil {
		return err
	}
	c.sbx = sbx
	c.owned = owned
	log.Debugf("daytona sandbox ready (lazy): id=%s name=%s owned=%v workspace=%s",
		sbx.ID, sbx.Name, owned, c.workspacePath)
	return nil
}

// stageInputsOnce stages the configured attachment inputs exactly once, the
// first time a workspace is created. Best-effort: a failure is reported (via
// the stage-error handler or a warn log) but never blocks code execution —
// the agent is instructed to tell the user when a staged file is missing.
func (c *CodeExecutor) stageInputsOnce(ctx context.Context, rt *workspaceRuntime, ws codeexecutor.Workspace) {
	if len(c.stageSpecs) == 0 {
		return
	}
	c.stageOnce.Do(func() {
		if err := rt.StageInputs(ctx, ws, c.stageSpecs); err != nil {
			if c.stageErrHandler != nil {
				c.stageErrHandler(err)
			} else {
				log.Warnf("daytona: stage attachments failed (continuing): %v", err)
			}
		}
	})
}

// acquireSandbox returns (sandbox, owned, error).
// owned=true: this executor created the sandbox, stop it on Close.
// owned=false: the sandbox was pre-existing, never stopped by this executor.
func (c *CodeExecutor) acquireSandbox(
	ctx context.Context,
	client *dtclient.Client,
) (*dtclient.Sandbox, bool, error) {
	timeout := c.sandboxTimeout
	if timeout <= 0 {
		timeout = defaultSandboxTimeout
	}
	createOpts := []func(*dtoptions.CreateSandbox){
		dtoptions.WithTimeout(timeout),
		dtoptions.WithWaitForStart(true),
	}

	var (
		sbx   *dtclient.Sandbox
		owned bool
	)

	switch {
	// 1. Connect to existing sandbox by explicit ID.
	case c.sandboxID != "":
		existing, err := client.Get(ctx, c.sandboxID)
		if err != nil {
			return nil, false, fmt.Errorf("daytona: get sandbox %s: %w", c.sandboxID, err)
		}
		if err := existing.StartWithTimeout(ctx, timeout); err != nil {
			log.Debugf("daytona: start existing sandbox (may already be running): %v", err)
		}
		sbx = existing

	// 2. Connect-or-create by name.
	case c.sandboxName != "":
		existing, getErr := client.Get(ctx, c.sandboxName)
		if getErr == nil && existing != nil {
			if err := existing.StartWithTimeout(ctx, timeout); err != nil {
				log.Debugf("daytona: start named sandbox (may already be running): %v", err)
			}
			sbx = existing
		} else {
			created, err := client.Create(ctx, c.buildCreateParams(), createOpts...)
			if err != nil {
				return nil, false, fmt.Errorf("daytona: create sandbox: %w", err)
			}
			sbx = created
			owned = true
		}

	// 3. No id or name — always create fresh.
	default:
		created, err := client.Create(ctx, c.buildCreateParams(), createOpts...)
		if err != nil {
			return nil, false, fmt.Errorf("daytona: create sandbox: %w", err)
		}
		sbx = created
		owned = true
	}

	// Apply the auto-archive interval on every path so reconnects pick up
	// updated values, not just freshly-created sandboxes.
	if c.autoStopMinutes != nil {
		if err := sbx.SetAutoArchiveInterval(ctx, c.autoStopMinutes); err != nil {
			log.Debugf("daytona: set auto-archive interval: %v", err)
		}
	}

	return sbx, owned, nil
}

// buildCreateParams returns the params for client.Create based on configuration.
func (c *CodeExecutor) buildCreateParams() any {
	base := dttypes.SandboxBaseParams{}
	if c.sandboxName != "" {
		base.Name = c.sandboxName
	}
	if c.envVars != nil {
		base.EnvVars = c.envVars
	}
	if c.labels != nil {
		base.Labels = c.labels
	}

	// Embed auto-stop interval into creation params if configured.
	if c.autoStopMinutes != nil {
		base.AutoStopInterval = c.autoStopMinutes
	}

	// Attach volumes (mounted at the executor's workspace path so the
	// agent's standard layout sits directly on the volume's subpath).
	if len(c.volumes) > 0 {
		mounts := make([]dttypes.VolumeMount, 0, len(c.volumes))
		for _, vm := range c.volumes {
			m := dttypes.VolumeMount{
				VolumeID:  vm.VolumeID,
				MountPath: c.workspacePath,
			}
			if vm.SubPath != "" {
				sp := vm.SubPath
				m.Subpath = &sp
			}
			mounts = append(mounts, m)
		}
		base.Volumes = mounts
	}

	if c.snapshot != "" {
		return dttypes.SnapshotParams{
			SandboxBaseParams: base,
			Snapshot:          c.snapshot,
		}
	}
	// Return nil (Daytona default image) when no params were customised.
	if c.sandboxName == "" && len(c.envVars) == 0 && len(c.labels) == 0 &&
		c.autoStopMinutes == nil && len(c.volumes) == 0 {
		return nil
	}
	return base
}

// ─── CodeExecutor interface ───────────────────────────────────────────────────

// CodeBlockDelimiter returns the fenced code delimiter.
func (c *CodeExecutor) CodeBlockDelimiter() codeexecutor.CodeBlockDelimiter {
	return codeexecutor.CodeBlockDelimiter{Start: "```", End: "```"}
}

// ExecuteCode writes each code block to the persistent workspace and runs it,
// aggregating output from all blocks. Suitable for the high-level code
// execution path (LLM emitting fenced blocks).
func (c *CodeExecutor) ExecuteCode(
	ctx context.Context,
	input codeexecutor.CodeExecutionInput,
) (codeexecutor.CodeExecutionResult, error) {
	// CreateWorkspace below lazily provisions the sandbox (ensureSandbox) on
	// first use; no eager nil check is needed.
	execID := input.ExecutionID
	if execID == "" {
		execID = "inline"
	}

	ws, err := c.ensureRuntime().CreateWorkspace(
		ctx, execID, codeexecutor.WorkspacePolicy{},
	)
	if err != nil {
		return codeexecutor.CodeExecutionResult{}, err
	}

	var allOut strings.Builder
	for i, block := range input.CodeBlocks {
		fn, mode, cmd, args, err := codeexecutor.BuildBlockSpec(i, block)
		if err != nil {
			fmt.Fprintf(&allOut, "[error] block %d: %s\n", i, err)
			continue
		}

		pf := codeexecutor.PutFile{
			Path:    fmt.Sprintf("%s/%s", codeexecutor.InlineSourceDir, fn),
			Content: []byte(block.Code),
			Mode:    mode,
		}
		if err := c.ensureRuntime().PutFiles(ctx, ws, []codeexecutor.PutFile{pf}); err != nil {
			fmt.Fprintf(&allOut, "[error] stage block %d: %s\n", i, err)
			continue
		}

		argv := append(append([]string{}, args...), "./"+fn)
		res, err := c.ensureRuntime().RunProgram(ctx, ws, codeexecutor.RunProgramSpec{
			Cmd:  cmd,
			Args: argv,
			Cwd:  codeexecutor.InlineSourceDir,
		})
		if err != nil {
			fmt.Fprintf(&allOut, "[error] run block %d: %s\n", i, err)
			continue
		}
		allOut.WriteString(res.Stdout)
		if res.Stderr != "" {
			allOut.WriteString(res.Stderr)
		}
	}

	return codeexecutor.CodeExecutionResult{Output: allOut.String()}, nil
}

// Engine exposes the persistent-workspace runtime as an Engine for skill tools
// (workspace_exec, workspace_save_artifact, workspaceio, etc.).
// This is required for workspace_exec tool to work.
func (c *CodeExecutor) Engine() codeexecutor.Engine {
	rt := c.ensureRuntime()
	return codeexecutor.NewEngine(rt, rt, rt)
}

// SandboxID returns the current sandbox ID.
func (c *CodeExecutor) SandboxID() string {
	if c.sbx == nil {
		return ""
	}
	return c.sbx.ID
}

// Sandbox exposes the underlying Daytona sandbox for advanced usage.
func (c *CodeExecutor) Sandbox() *dtclient.Sandbox { return c.sbx }

// Close releases this executor's resources. The sandbox lifecycle action
// depends on construction options:
//
//   - WithDeleteOnClose: the sandbox is deleted (ephemeral lifecycle, used
//     when durable state lives on an attached volume).
//   - Otherwise: the sandbox is stopped only when this executor created it
//     (owned=true). Pre-existing sandboxes are left untouched so their
//     filesystem persists for future sessions.
//
// The SDK client is always closed to release its HTTP connection pool.
func (c *CodeExecutor) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var firstErr error
	if c.sbx != nil {
		switch {
		case c.deleteOnClose:
			if err := c.sbx.Delete(context.Background()); err != nil {
				log.Debugf("daytona: delete sandbox %s: %v", c.sbx.ID, err)
				firstErr = err
			}
		case c.owned:
			if err := c.sbx.Stop(context.Background()); err != nil {
				log.Debugf("daytona: stop sandbox %s: %v", c.sbx.ID, err)
				firstErr = err
			}
		}
	}
	if c.client != nil {
		if err := c.client.Close(context.Background()); err != nil && firstErr == nil {
			log.Debugf("daytona: close client: %v", err)
			firstErr = err
		}
	}
	c.sbx = nil
	c.client = nil
	return firstErr
}

// EnsureVolume returns the ID of a Daytona volume with the given name,
// creating it if it does not exist and waiting up to readyTimeout for it
// to become ready. It is safe to call concurrently from multiple requests:
// the SDK's Create returns a conflict error which we translate into a Get.
//
// readyTimeout defaults to 2 minutes when ≤ 0.
//
// This is a thin wrapper around the SDK that hides dtclient-specific types
// from NixFlow callers so they don't need to import the Daytona SDK directly.
func EnsureVolume(ctx context.Context, apiKey, apiURL, name string, readyTimeout time.Duration) (string, error) {
	if name == "" {
		return "", errors.New("daytona: volume name required")
	}
	if readyTimeout <= 0 {
		readyTimeout = 2 * time.Minute
	}

	cfg := &dttypes.DaytonaConfig{}
	if apiKey != "" {
		cfg.APIKey = apiKey
	}
	if apiURL != "" {
		cfg.APIUrl = apiURL
	}

	client, err := dtclient.NewClientWithConfig(cfg)
	if err != nil {
		return "", fmt.Errorf("daytona: ensure volume: create client: %w", err)
	}
	defer func() { _ = client.Close(context.Background()) }()

	if existing, getErr := client.Volume.Get(ctx, name); getErr == nil && existing != nil {
		if existing.State != "ready" {
			existing, err = client.Volume.WaitForReady(ctx, existing, readyTimeout)
			if err != nil {
				return "", fmt.Errorf("daytona: wait for volume %q ready: %w", name, err)
			}
		}
		return existing.ID, nil
	}

	created, err := client.Volume.Create(ctx, name)
	if err != nil {
		// Concurrent creators may race; fall back to Get.
		if existing, getErr := client.Volume.Get(ctx, name); getErr == nil && existing != nil {
			if existing.State != "ready" {
				existing, _ = client.Volume.WaitForReady(ctx, existing, readyTimeout)
			}
			if existing != nil {
				return existing.ID, nil
			}
		}
		return "", fmt.Errorf("daytona: create volume %q: %w", name, err)
	}
	if created.State != "ready" {
		created, err = client.Volume.WaitForReady(ctx, created, readyTimeout)
		if err != nil {
			return "", fmt.Errorf("daytona: wait for volume %q ready: %w", name, err)
		}
	}
	return created.ID, nil
}

func (c *CodeExecutor) ensureRuntime() *workspaceRuntime {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rt == nil {
		c.rt = newWorkspaceRuntime(c)
	}
	return c.rt
}
