package setting

import (
	"strings"
	"testing"
)

// validReleaseConfig returns a ServerConfig that passes Validate in release mode.
func validReleaseConfig() ServerConfig {
	var c ServerConfig
	c.System.Mode = "release"
	c.Jwt.AccessSigningKey = "prod-access-key"
	c.Jwt.RefreshSigningKey = "prod-refresh-key"
	c.DB.Password = "prod-db-pass"
	c.System.SuperUserPassword = "prod-admin-pass"
	c.Nomad.Host = "http://nomad-1"
	c.Agent.Daytona.DefaultSnapshot = "antelope-bio"
	c.Agent.Daytona.WorkspacePath = "/home/user/trpc_agent_workspace"
	return c
}

func TestValidate_ReleaseFailsFast(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ServerConfig)
		wantEnv string
	}{
		{"missing access key", func(c *ServerConfig) { c.Jwt.AccessSigningKey = "" }, "ANTELOPE_JWT_ACCESS_SIGNING_KEY"},
		{"default access key", func(c *ServerConfig) { c.Jwt.AccessSigningKey = "stjudecab@access" }, "ANTELOPE_JWT_ACCESS_SIGNING_KEY"},
		{"missing refresh key", func(c *ServerConfig) { c.Jwt.RefreshSigningKey = "" }, "ANTELOPE_JWT_REFRESH_SIGNING_KEY"},
		{"missing db password", func(c *ServerConfig) { c.DB.Password = "" }, "ANTELOPE_POSTGRESQL_PASSWORD"},
		{"default super-user password", func(c *ServerConfig) { c.System.SuperUserPassword = "password" }, "ANTELOPE_SYSTEM_SUPER_USER_PASSWORD"},
		{"missing nomad host", func(c *ServerConfig) { c.Nomad.Host = "" }, "ANTELOPE_NOMAD_HOST"},
		{"relative workspace path", func(c *ServerConfig) { c.Agent.Daytona.WorkspacePath = "relative/path" }, "workspace-path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validReleaseConfig()
			tt.mutate(&c)
			err := c.Validate()
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantEnv) {
				t.Errorf("error %q does not mention %q", err.Error(), tt.wantEnv)
			}
		})
	}
}

func TestValidate_ReleaseValidPasses(t *testing.T) {
	c := validReleaseConfig()
	if err := c.Validate(); err != nil {
		t.Fatalf("expected a valid release config to pass, got: %v", err)
	}
}

func TestValidate_DebugNeverFatal(t *testing.T) {
	// Everything empty/default in debug mode must warn but never error.
	c := ServerConfig{}
	c.System.Mode = "debug"
	if err := c.Validate(); err != nil {
		t.Fatalf("debug mode should not return an error, got: %v", err)
	}
}

func TestRedact(t *testing.T) {
	for _, empty := range []string{"", "   "} {
		if got := redact(empty); got != "(empty)" {
			t.Errorf("redact(%q) = %q, want (empty)", empty, got)
		}
	}
	if got := redact("sk-secret"); got != "***" {
		t.Errorf("redact(secret) = %q, want ***", got)
	}
}

func TestSummary_RedactsSecrets(t *testing.T) {
	c := validReleaseConfig()
	c.DB.Password = "super-secret-db"
	c.Jwt.AccessSigningKey = "super-secret-jwt"
	s := c.Summary()
	if strings.Contains(s, "super-secret-db") || strings.Contains(s, "super-secret-jwt") {
		t.Errorf("summary leaked a secret value:\n%s", s)
	}
	if !strings.Contains(s, "***") {
		t.Errorf("summary did not redact any secret:\n%s", s)
	}
}
