package llmconfig

import (
	"encoding/json"
	"strings"
	"testing"

	"antelope/pkg/secretbox"
)

func newTestBox(t *testing.T) *secretbox.Box {
	t.Helper()
	keyB64, err := secretbox.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	box, err := secretbox.New(keyB64)
	if err != nil {
		t.Fatalf("new box: %v", err)
	}
	return box
}

// With a key configured, the Redis cache payload must be ciphertext (no plaintext
// API key) and must round-trip back to the original configs.
func TestCacheCodec_Encrypted(t *testing.T) {
	m := &Manager{box: newTestBox(t)}
	in := []UserLLMConfig{{ID: "1", Provider: "openai", APIKey: "sk-super-secret"}}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}

	enc, err := m.encodeForCache(raw)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if strings.Contains(enc, "sk-super-secret") {
		t.Fatal("cache payload leaks plaintext API key")
	}

	out, err := m.decodeFromCache(enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 1 || out[0].APIKey != "sk-super-secret" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

// Without a key (box == nil) the cache stays plaintext, preserving the prior
// behaviour for key-less deployments.
func TestCacheCodec_PlaintextWhenNoKey(t *testing.T) {
	m := &Manager{}
	in := []UserLLMConfig{{ID: "1", APIKey: "k"}}
	raw, _ := json.Marshal(in)

	enc, err := m.encodeForCache(raw)
	if err != nil {
		t.Fatal(err)
	}
	if enc != string(raw) {
		t.Fatalf("expected plaintext, got %q", enc)
	}
	out, err := m.decodeFromCache(enc)
	if err != nil || len(out) != 1 {
		t.Fatalf("decode plaintext: %v %+v", err, out)
	}
}

// A key-enabled manager must still read legacy plaintext cache values written
// before encryption was enabled, so a rolling deploy needn't flush Redis.
func TestCacheCodec_LegacyPlaintextWithBox(t *testing.T) {
	m := &Manager{box: newTestBox(t)}
	in := []UserLLMConfig{{ID: "1", APIKey: "legacy"}}
	raw, _ := json.Marshal(in)

	out, err := m.decodeFromCache(string(raw))
	if err != nil {
		t.Fatalf("decode legacy plaintext: %v", err)
	}
	if len(out) != 1 || out[0].APIKey != "legacy" {
		t.Fatalf("legacy round-trip mismatch: %+v", out)
	}
}
