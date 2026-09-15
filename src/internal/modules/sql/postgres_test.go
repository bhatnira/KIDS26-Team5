package sql

import (
	"strings"
	"testing"

	"antelope/internal/modules/setting"
)

// TestRedactDSN verifies that the password is masked while every other field
// (host, user, dbname, port, sslmode) is preserved for debuggability — so the
// DSN can be safely embedded in ping-failure errors and panic logs.
func TestRedactDSN(t *testing.T) {
	const password = "s3cr3t-p@ss=word"
	dsn := ToPostgresDSN(setting.DBConfig{
		Host:     "db.internal",
		Username: "app",
		Password: password,
		DB:       "antelope",
		Port:     5432,
		SSLMode:  "require",
	})

	// Precondition: the raw DSN must contain the plaintext password.
	if !strings.Contains(dsn, password) {
		t.Fatalf("precondition failed: raw DSN should contain password, got %q", dsn)
	}

	red := RedactDSN(dsn)

	if strings.Contains(red, password) {
		t.Fatalf("redacted DSN still contains plaintext password: %q", red)
	}
	if !strings.Contains(red, "password=xxxxx") {
		t.Fatalf("expected masked password marker, got %q", red)
	}
	for _, want := range []string{
		"host=db.internal", "user=app", "dbname=antelope", "port=5432", "sslmode=require",
	} {
		if !strings.Contains(red, want) {
			t.Errorf("redacted DSN missing %q: %q", want, red)
		}
	}
}

// TestRedactDSNEmptyPassword ensures an empty password field is still masked and
// the call does not panic.
func TestRedactDSNEmptyPassword(t *testing.T) {
	dsn := ToPostgresDSN(setting.DBConfig{Host: "h", Username: "u", DB: "d", Port: 5432})
	red := RedactDSN(dsn)
	if !strings.Contains(red, "password=xxxxx") {
		t.Errorf("expected masked password even when empty, got %q", red)
	}
}
