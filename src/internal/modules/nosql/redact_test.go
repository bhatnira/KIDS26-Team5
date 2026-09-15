package nosql

import (
	"strings"
	"testing"

	"antelope/internal/modules/setting"
)

// TestRedactRedisURI verifies the password is masked while host/port/db survive,
// so the canonical URI can be safely embedded in ping-failure errors and panic
// logs. RedisConfig.URI() uses url.UserPassword, and url.URL.String() emits the
// password in plaintext — redactRedisURI must close that gap.
func TestRedactRedisURI(t *testing.T) {
	const password = "sup3r-s3cret"
	cfg := setting.RedisConfig{Host: "redis.internal", Port: 6379, DB: 2, Password: password}
	uri := cfg.URI()

	// Precondition: the canonical URI must carry the plaintext password.
	if !strings.Contains(uri, password) {
		t.Fatalf("precondition failed: canonical URI should contain password, got %q", uri)
	}

	red := redactRedisURI(uri)

	if strings.Contains(red, password) {
		t.Fatalf("redacted URI still contains plaintext password: %q", red)
	}
	if !strings.Contains(red, "redis.internal:6379") {
		t.Errorf("redacted URI missing host:port: %q", red)
	}
	if !strings.Contains(red, "/2") {
		t.Errorf("redacted URI missing db index: %q", red)
	}
}

// TestRedactRedisURINoPassword ensures a password-less URI passes through with
// host/port preserved.
func TestRedactRedisURINoPassword(t *testing.T) {
	cfg := setting.RedisConfig{Host: "redis.internal", Port: 6379, DB: 0}
	red := redactRedisURI(cfg.URI())
	if !strings.Contains(red, "redis.internal:6379") {
		t.Errorf("expected host:port preserved, got %q", red)
	}
}

// TestRedactRedisURIUnparseable ensures an unparseable input never echoes a
// would-be secret back to the caller.
func TestRedactRedisURIUnparseable(t *testing.T) {
	red := redactRedisURI("redis://:%zzsecret@host:6379/0")
	if strings.Contains(red, "secret") {
		t.Fatalf("redaction leaked input on parse failure: %q", red)
	}
	if red == "" {
		t.Error("expected non-empty fallback")
	}
}
