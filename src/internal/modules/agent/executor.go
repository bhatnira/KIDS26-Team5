package agent

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"antelope/internal/modules/agent/daytona"
	"antelope/internal/modules/setting"
	"antelope/models"

	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

const (
	defaultDaytonaSandboxTimeoutSec = 120
	defaultDaytonaWorkspacePath     = "/home/user/trpc_agent_workspace"
)

// SessionExecutorResult describes what BuildSessionExecutor produced. With
// lazy provisioning the sandbox does not exist yet, so the live sandbox id
// is read from the executor at close time, not stored here.
type SessionExecutorResult struct {
	Executor *daytona.CodeExecutor
}

// SessionExecutorParams bundles the per-turn inputs for BuildSessionExecutor.
type SessionExecutorParams struct {
	Cfg       setting.AgentConfig
	WS        models.AgentWorkspaceConfig
	UserID    uint
	SessionID string

	// Fetcher resolves s3buf:// / s3url:// attachment inputs; nil disables
	// attachment staging for the turn.
	Fetcher daytona.ObjectFetcher
	// StageSpecs are staged into the workspace on the first code execution.
	StageSpecs []codeexecutor.InputSpec
	// OnStageErr receives a best-effort staging failure (logged with context).
	OnStageErr func(error)
}

// BuildSessionExecutor returns a LAZY Daytona executor for one chat turn: it
// allocates no sandbox and makes no Daytona API calls until the agent first
// executes code. A turn that only answers in text (or reads an inlined
// attachment) therefore never provisions a sandbox.
//
// Workspace location — local, NOT an S3 volume.
//
// The agent workspace runs on the sandbox's local filesystem. We deliberately
// do NOT mount the per-user S3-backed Daytona volume as the workspace: those
// volumes use Mountpoint-for-S3 over a general-purpose bucket (MinIO), which
// cannot perform the POSIX operations the agent and the framework rely on —
// file rename (used to atomically commit metadata.json), symlinks (skill
// staging links work/out under skills/<name>), chmod, and in-place edits are
// all unsupported on general-purpose buckets. Running the workspace there
// fails immediately, e.g.:
//
//	workspace metadata commit failed: exit code 1
//
// Consequence: the sandbox is ephemeral per turn (WithDeleteOnClose), so
// scratch files do not persist across turns. Durable results still persist —
// artifacts are saved through the artifact service (object storage) and
// restored at their original workspace paths every turn (see
// sessionArtifactSpecs), and chat attachments from every turn of the
// conversation are re-staged from the user's bucket (see
// SessionStateStagedAttachments). (To restore full cross-turn workspace
// persistence one would need a POSIX-capable volume, e.g. an S3 Express One
// Zone directory bucket, or a long-lived per-conversation sandbox.)
func BuildSessionExecutor(p SessionExecutorParams) (*SessionExecutorResult, error) {
	apiKey := strings.TrimSpace(p.WS.DaytonaAPIKey)
	if apiKey == "" {
		return nil, errors.New("Daytona API key is not configured for this user")
	}
	apiURL := strings.TrimSpace(p.WS.DaytonaAPIURL)

	opts := baseDaytonaOpts(p.Cfg.Daytona, apiURL, apiKey)
	opts = append(opts,
		daytona.WithLazyStart(),
		daytona.WithDeleteOnClose(),
	)
	if p.Fetcher != nil {
		opts = append(opts, daytona.WithObjectFetcher(p.Fetcher))
	}
	if len(p.StageSpecs) > 0 {
		opts = append(opts, daytona.WithStageInputs(p.StageSpecs))
	}
	if p.OnStageErr != nil {
		opts = append(opts, daytona.WithStageErrorHandler(p.OnStageErr))
	}

	// daytona.New with lazy start only constructs the client + executor; the
	// sandbox is provisioned on first code execution.
	exec, err := daytona.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("create daytona executor: %w", err)
	}
	return &SessionExecutorResult{Executor: exec}, nil
}

func baseDaytonaOpts(cfg setting.AgentDaytonaConfig, apiURL, apiKey string) []daytona.Option {
	sboxTimeout := cfg.SandboxTimeoutSec
	if sboxTimeout <= 0 {
		sboxTimeout = defaultDaytonaSandboxTimeoutSec
	}
	wsPath := strings.TrimSpace(cfg.WorkspacePath)
	if wsPath == "" {
		wsPath = defaultDaytonaWorkspacePath
	}

	opts := []daytona.Option{
		daytona.WithAPIKey(apiKey),
		daytona.WithWorkspacePath(wsPath),
		daytona.WithSandboxTimeout(time.Duration(sboxTimeout) * time.Second),
	}
	if url := strings.TrimSpace(apiURL); url != "" {
		opts = append(opts, daytona.WithAPIURL(url))
	}
	if snap := strings.TrimSpace(cfg.DefaultSnapshot); snap != "" {
		opts = append(opts, daytona.WithSnapshot(snap))
	}
	// AutoStopMinutes is a safety net under the ephemeral per-turn
	// lifecycle (sandboxes don't normally live long enough to idle).
	if cfg.AutoStopMinutes > 0 {
		opts = append(opts, daytona.WithAutoStopInterval(cfg.AutoStopMinutes))
	}
	// DEV ONLY: let in-sandbox transfers accept self-signed storage certs.
	if cfg.InsecureTLSTransfers {
		opts = append(opts, daytona.WithInsecureTLSTransfers())
	}
	return opts
}
