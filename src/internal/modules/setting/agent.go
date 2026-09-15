package setting

import (
	"fmt"
	"strings"
)

// AgentConfig holds platform-wide defaults for the trpc-agent-go integration.
// Per-user settings (LLM provider, MCP servers, workspace bucket, runtime,
// credentials) live in the database; this config only carries server-side
// defaults shared across all users.
type AgentConfig struct {
	// Daytona sandbox defaults. Per-user API keys override these. Also
	// carries the platform-built snapshot name that ships the
	// bioinformatics runtime (see scripts/daytona/build-snapshot.sh).
	Daytona AgentDaytonaConfig `mapstructure:"daytona" json:"daytona" yaml:"daytona"`

	// Skills directories. The built-in library is either embedded in the
	// binary or read from BundleRoot; user skills are lazy-synced from S3 to
	// UserCacheRoot under {userID}/.
	Skills AgentSkillsConfig `mapstructure:"skills" json:"skills" yaml:"skills"`

	// Artifacts holds tunables for the per-user S3 artifact backend.
	Artifacts AgentArtifactsConfig `mapstructure:"artifacts" json:"artifacts" yaml:"artifacts"`

	// AppName is the framework's application namespace. Sessions, artifacts,
	// and tracks are scoped by this name. Defaults to "antelope".
	AppName string `mapstructure:"app-name" json:"app-name" yaml:"app-name"`
}

// AgentDaytonaConfig collects Daytona sandbox tunables.
//
// IMPORTANT: DefaultSnapshot and WorkspacePath are coupled to the sandbox
// image built by scripts/daytona/build-snapshot.sh — they are NOT free-form.
// DefaultSnapshot must name a snapshot that has actually been registered with
// Daytona (the script's SNAPSHOT_NAME, default "antelope-bio"), and
// WorkspacePath must match the directory tree baked into that image's
// Dockerfile. Changing either here without rebuilding/re-registering the
// snapshot makes sandbox creation fail at runtime.
type AgentDaytonaConfig struct {
	// DefaultSnapshot is the platform-built snapshot ID/name used when
	// creating a new sandbox. It must equal the SNAPSHOT_NAME registered by
	// scripts/daytona/build-snapshot.sh (default "antelope-bio"). Empty =
	// the Daytona-default image, which lacks the bioinformatics stack — the
	// agent logs a loud startup warning in that case.
	DefaultSnapshot string `mapstructure:"default-snapshot" json:"default-snapshot" yaml:"default-snapshot"`

	// WorkspacePath is the directory inside the sandbox where the persistent
	// agent workspace lives. It MUST match the path pre-created (and owned by
	// the `user` account) in scripts/daytona/Dockerfile — currently
	// /home/user/trpc_agent_workspace. Override only in lock-step with a
	// rebuilt snapshot; an absolute path is required.
	WorkspacePath string `mapstructure:"workspace-path" json:"workspace-path" yaml:"workspace-path"`

	// SandboxTimeoutSec bounds how long sandbox start may take.
	SandboxTimeoutSec int `mapstructure:"sandbox-timeout-sec" json:"sandbox-timeout-sec" yaml:"sandbox-timeout-sec"`

	// AutoStopMinutes archives the sandbox after this many idle minutes.
	// 0 disables auto-archive. Recommended: 30.
	AutoStopMinutes int `mapstructure:"auto-stop-minutes" json:"auto-stop-minutes" yaml:"auto-stop-minutes"`

	// InsecureTLSTransfers makes in-sandbox transfers (the output-harvest
	// presigned PUT and large-attachment presigned GET) skip TLS certificate
	// verification (curl -k / wget --no-check-certificate).
	//
	// DEV ONLY — TEMPORARY. This exists so a local docker-compose MinIO with
	// a self-signed cert can use the direct streaming path instead of the
	// API-process fallback. It disables TLS trust for sandbox→storage
	// transfers and MUST NOT be enabled in production; give the sandbox a
	// trusted cert instead. Default false.
	InsecureTLSTransfers bool `mapstructure:"insecure-tls-transfers" json:"insecure-tls-transfers" yaml:"insecure-tls-transfers"`
}

// AgentSkillsConfig collects skill repository locations.
//
// The built-in library (~700 bioinformatics skills pruned from the upstream
// submodules under skills/upstream/) reaches the runtime one of two ways:
//
//   - Release and container builds are compiled with `-tags skills`, which bakes
//     the generated bundle into the binary. On start-up it is unpacked once into
//     BundleCacheRoot; BundleRoot is then unused. Unpacking is not optional —
//     the framework stages a skill by copying its directory into the sandbox, so
//     the files have to exist on a real filesystem.
//   - Builds without that tag (`make dev`, `go test`) read the generated bundle
//     straight from BundleRoot. A missing bundle there is not an error; the agent
//     simply has no built-in skills until `make skills-bundle` has been run.
type AgentSkillsConfig struct {
	// BundleRoot is the generated skill bundle directory to read when the
	// binary carries no embedded copy. It must be a bundler output (a
	// directory containing index.json), not an arbitrary tree of skills.
	// Empty falls back to the bundler's default output path.
	BundleRoot string `mapstructure:"bundle-root" json:"bundle-root" yaml:"bundle-root"`

	// BundleCacheRoot is where an embedded bundle is unpacked. Point it at a
	// persistent path so restarts skip the ~23 MiB copy; unpacking is
	// content-hash gated, so an up-to-date directory is reused as-is. Empty
	// falls back to a directory under the OS temp dir.
	BundleCacheRoot string `mapstructure:"bundle-cache-root" json:"bundle-cache-root" yaml:"bundle-cache-root"`

	// UserCacheRoot is the host directory where per-user skill bundles are
	// lazy-synced from object storage. Subdir per userID.
	UserCacheRoot string `mapstructure:"user-cache-root" json:"user-cache-root" yaml:"user-cache-root"`
}

// summary renders the skill roots for the start-up config banner. Which root is
// live depends on the build tags, so both are shown, with "(default)" standing in
// for the values skillbundle picks when they are unset.
func (c AgentSkillsConfig) summary() string {
	or := func(v string) string {
		if strings.TrimSpace(v) == "" {
			return "(default)"
		}
		return v
	}
	return fmt.Sprintf("bundle=%s cache=%s user=%s",
		or(c.BundleRoot), or(c.BundleCacheRoot), or(c.UserCacheRoot))
}

// AgentArtifactsConfig collects artifact storage tunables.
type AgentArtifactsConfig struct {
	// PresignedTTLSeconds controls how long a download URL stays valid.
	PresignedTTLSeconds int `mapstructure:"presigned-ttl-seconds" json:"presigned-ttl-seconds" yaml:"presigned-ttl-seconds"`
}
