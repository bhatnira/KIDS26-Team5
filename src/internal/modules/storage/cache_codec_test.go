package storage

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

func sampleEnvelope() (ProviderConfig, []byte) {
	env := ProviderConfig{
		Type:      ProviderMinio,
		RawConfig: json.RawMessage(`{"access_key":"AKIA","secret_key":"s3cr3t-key"}`),
		Hash:      "h1",
	}
	raw, _ := json.Marshal(env)
	return env, raw
}

// With a key configured, the Redis cache payload must be ciphertext (no plaintext
// secret key) and must round-trip back to the original envelope.
func TestStorageCacheCodec_Encrypted(t *testing.T) {
	m := &ClientManager{box: newTestBox(t)}
	_, raw := sampleEnvelope()

	enc, err := m.encodeConfigForCache(raw)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if strings.Contains(enc, "s3cr3t-key") {
		t.Fatal("cache payload leaks plaintext secret key")
	}

	out, err := m.decodeConfigFromCache(enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Hash != "h1" || !strings.Contains(string(out.RawConfig), "s3cr3t-key") {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

// Without a key (box == nil) the cache stays plaintext.
func TestStorageCacheCodec_PlaintextWhenNoKey(t *testing.T) {
	m := &ClientManager{}
	_, raw := sampleEnvelope()

	enc, err := m.encodeConfigForCache(raw)
	if err != nil {
		t.Fatal(err)
	}
	if enc != string(raw) {
		t.Fatalf("expected plaintext, got %q", enc)
	}
	if _, err := m.decodeConfigFromCache(enc); err != nil {
		t.Fatalf("decode plaintext: %v", err)
	}
}

// A key-enabled manager must still read legacy plaintext cache values.
func TestStorageCacheCodec_LegacyPlaintextWithBox(t *testing.T) {
	m := &ClientManager{box: newTestBox(t)}
	_, raw := sampleEnvelope()

	out, err := m.decodeConfigFromCache(string(raw))
	if err != nil {
		t.Fatalf("decode legacy plaintext: %v", err)
	}
	if out.Hash != "h1" {
		t.Fatalf("legacy round-trip mismatch: %+v", out)
	}
}
