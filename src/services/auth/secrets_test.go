package auth

import (
	"strings"
	"testing"

	"antelope/pkg/secretbox"
)

func newBox(t *testing.T) *secretbox.Box {
	t.Helper()
	key, err := secretbox.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	box, err := secretbox.New(key)
	if err != nil {
		t.Fatalf("new box: %v", err)
	}
	return box
}

func TestSecretRoundTrip(t *testing.T) {
	s := &authService{box: newBox(t)}

	const plain = "s3cr3t-bind-pass"
	enc, err := s.encryptSecret(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !strings.HasPrefix(enc, encPrefix) {
		t.Fatalf("ciphertext missing %q prefix: %q", encPrefix, enc)
	}
	if enc == plain || strings.Contains(enc, plain) {
		t.Fatalf("ciphertext leaks plaintext: %q", enc)
	}

	got, err := s.decryptSecret(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plain {
		t.Fatalf("round-trip mismatch: got %q want %q", got, plain)
	}
}

func TestDecryptLegacyPlaintextPassthrough(t *testing.T) {
	// Rows written before encryption (no prefix) must read back unchanged,
	// whether or not a box is configured.
	for _, s := range []*authService{{box: newBox(t)}, {box: nil}} {
		const legacy = "legacy-plaintext-secret"
		got, err := s.decryptSecret(legacy)
		if err != nil {
			t.Fatalf("decrypt legacy: %v", err)
		}
		if got != legacy {
			t.Fatalf("legacy passthrough mismatch: got %q want %q", got, legacy)
		}
	}
}

func TestEncryptWithoutBoxIsPlaintext(t *testing.T) {
	s := &authService{box: nil}
	const plain = "no-key-configured"
	enc, err := s.encryptSecret(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc != plain {
		t.Fatalf("expected plaintext passthrough, got %q", enc)
	}
}

func TestEncryptEmptyStaysEmpty(t *testing.T) {
	s := &authService{box: newBox(t)}
	enc, err := s.encryptSecret("")
	if err != nil {
		t.Fatalf("encrypt empty: %v", err)
	}
	if enc != "" {
		t.Fatalf("empty secret should not be encrypted, got %q", enc)
	}
}

func TestDecryptEncryptedWithoutBoxErrors(t *testing.T) {
	// A prefixed (encrypted) value can't be read once the key is gone — surface
	// an error rather than returning corrupt bytes.
	enc, err := (&authService{box: newBox(t)}).encryptSecret("x")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := (&authService{box: nil}).decryptSecret(enc); err == nil {
		t.Fatal("expected error decrypting ciphertext with no box, got nil")
	}
}
