package setting

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// insecureDefaults are the placeholder secrets shipped in the example configs
// and built-in fallbacks. Using any of them in release mode is treated as
// "unset" so a deployment can't accidentally ship with a public signing key or
// a well-known password.
var insecureDefaults = map[string]struct{}{
	"":                   {},
	"password":           {},
	"admin123":           {},
	"changeme":           {},
	"stjudecab@access":   {},
	"stjudecab@refresh":  {},
	"stjudecab@postgres": {},
	"stjudecab@redis":    {},
	"antelope@access":    {},
	"antelope@refresh":   {},
}

func isInsecure(v string) bool {
	_, bad := insecureDefaults[strings.TrimSpace(v)]
	return bad
}

// Validate checks the loaded configuration for missing or insecure values.
//
// In release mode every problem in the required set is fatal and returned as a
// single aggregated error naming the ANTELOPE_* env var to set. In debug mode
// the same problems are downgraded to warnings so local development keeps
// working with the shipped defaults. Agent/sandbox sanity checks (snapshot
// present, absolute workspace path, skills dir reachable) apply in both modes.
func (c *ServerConfig) Validate() error {
	var problems []string
	require := func(ok bool, env, msg string) {
		if !ok {
			problems = append(problems, fmt.Sprintf("  - %s: %s", env, msg))
		}
	}

	// Per-deployment secrets that must never run on a shipped default.
	require(!isInsecure(c.Jwt.AccessSigningKey), "ANTELOPE_JWT_ACCESS_SIGNING_KEY",
		"JWT access signing key is empty or a known default")
	require(!isInsecure(c.Jwt.RefreshSigningKey), "ANTELOPE_JWT_REFRESH_SIGNING_KEY",
		"JWT refresh signing key is empty or a known default")
	require(strings.TrimSpace(c.DB.Password) != "", "ANTELOPE_POSTGRESQL_PASSWORD",
		"PostgreSQL password is empty")
	require(!isInsecure(c.System.SuperUserPassword), "ANTELOPE_SYSTEM_SUPER_USER_PASSWORD",
		"super-user password is empty or a known default")
	require(strings.TrimSpace(c.Nomad.Host) != "", "ANTELOPE_NOMAD_HOST",
		"Nomad host is empty")

	// A non-absolute workspace path can never match the snapshot image, so it
	// is always a hard problem (fatal in release, warning in debug).
	if ws := strings.TrimSpace(c.Agent.Daytona.WorkspacePath); ws != "" && !filepath.IsAbs(ws) {
		problems = append(problems, fmt.Sprintf("  - agent.daytona.workspace-path: %q must be an absolute path", ws))
	}

	// Soft warnings — surfaced in both modes, never fatal. The zap logger is
	// not initialised yet when InitConfig runs, so these go straight to stderr.
	if strings.TrimSpace(c.Agent.Daytona.DefaultSnapshot) == "" {
		fmt.Fprintln(os.Stderr, "[config] warning: agent.daytona.default-snapshot is empty; "+
			"sandboxes fall back to the base Daytona image, which lacks the bioinformatics "+
			"stack (build one via scripts/daytona/build-snapshot.sh)")
	}
	// The bundle root only matters to builds without an embedded library, and
	// the agent reports what it actually loaded at start-up, so this is not
	// checked here — an absent bundle is a supported development state.
	if dir := strings.TrimSpace(c.Agent.Skills.BundleCacheRoot); dir != "" && !filepath.IsAbs(dir) {
		fmt.Fprintf(os.Stderr, "[config] warning: agent.skills.bundle-cache-root %q is relative; "+
			"an embedded skill bundle would unpack relative to the process working "+
			"directory, which differs between `make dev` and the container\n", dir)
	}
	if strings.TrimSpace(c.System.EncryptKey) == "" && c.System.IsProduction() {
		fmt.Fprintln(os.Stderr, "[config] warning: system.encrypt-key is unset; per-user storage/LLM "+
			"credentials are persisted UNENCRYPTED. Set ANTELOPE_SYSTEM_ENCRYPT_KEY "+
			"(e.g. `openssl rand -base64 32`) to encrypt them at rest.")
	}

	if len(problems) == 0 {
		return nil
	}

	body := "configuration validation failed; set the following (env vars override config.yaml):\n" +
		strings.Join(problems, "\n")
	if c.System.IsProduction() {
		return errors.New(body)
	}
	fmt.Fprintf(os.Stderr, "[config] warning (debug mode — not fatal): %s\n", body)
	return nil
}

// redact masks a secret for log output while still revealing whether it is set.
func redact(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(empty)"
	}
	return "***"
}

// Summary returns a human-readable, secret-redacted snapshot of the effective
// configuration for startup logging and troubleshooting.
func (c *ServerConfig) Summary() string {
	var b strings.Builder
	b.WriteString("config summary (secrets redacted):\n")
	fmt.Fprintf(&b, "  system:   mode=%s port=%d super-user=%s encrypt-key=%s\n",
		c.System.Mode, c.System.Port, c.System.SuperUser, redact(c.System.EncryptKey))
	fmt.Fprintf(&b, "  postgres: %s:%d/%s user=%s password=%s sslmode=%s\n",
		c.DB.Host, c.DB.Port, c.DB.DB, c.DB.Username, redact(c.DB.Password), c.DB.sslMode())
	fmt.Fprintf(&b, "  redis:    %s:%d db=%d password=%s\n",
		c.Redis.Host, c.Redis.Port, c.Redis.DB, redact(c.Redis.Password))
	fmt.Fprintf(&b, "  nomad:    %s:%d ns=%s region=%s token=%s\n",
		c.Nomad.Host, c.Nomad.Port, c.Nomad.Namespace, c.Nomad.Region, redact(c.Nomad.Token))
	fmt.Fprintf(&b, "  jwt:      issuer=%s access-key=%s refresh-key=%s\n",
		c.Jwt.Issuer, redact(c.Jwt.AccessSigningKey), redact(c.Jwt.RefreshSigningKey))
	fmt.Fprintf(&b, "  email:    %s:%d from=%s auth=%s tls=%s password=%s\n",
		c.Email.Host, c.Email.Port, c.Email.From, c.Email.AuthType, c.Email.TLSPolicy, redact(c.Email.Password))
	fmt.Fprintf(&b, "  agent:    snapshot=%s workspace=%s skills=%s\n",
		c.Agent.Daytona.DefaultSnapshot, c.Agent.Daytona.WorkspacePath, c.Agent.Skills.summary())
	fmt.Fprintf(&b, "  cab:      base-url=%s\n", c.Cab.BaseURL)
	return b.String()
}
