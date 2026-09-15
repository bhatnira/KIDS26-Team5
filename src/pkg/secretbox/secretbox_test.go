package secretbox

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func newTestBox(t *testing.T) *Box {
	t.Helper()
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	b, err := New(key)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return b
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	b := newTestBox(t)
	plain := []byte(`{"accessKey":"AKIA...","secretKey":"s3cr3t"}`)

	enc, err := b.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if strings.Contains(enc, "s3cr3t") {
		t.Fatal("ciphertext leaks plaintext")
	}

	got, err := b.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("round trip mismatch: got %q want %q", got, plain)
	}
}

func TestEncryptIsNonDeterministic(t *testing.T) {
	b := newTestBox(t)
	a, _ := b.Encrypt([]byte("same"))
	c, _ := b.Encrypt([]byte("same"))
	if a == c {
		t.Fatal("expected distinct ciphertexts (random nonce)")
	}
}

func TestDecryptRejectsTampering(t *testing.T) {
	b := newTestBox(t)
	enc, _ := b.Encrypt([]byte("payload"))
	raw, _ := base64.StdEncoding.DecodeString(enc)
	raw[len(raw)-1] ^= 0xff // flip a tag bit
	if _, err := b.Decrypt(base64.StdEncoding.EncodeToString(raw)); err == nil {
		t.Fatal("expected authentication failure on tampered ciphertext")
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	b1 := newTestBox(t)
	b2 := newTestBox(t)
	enc, _ := b1.Encrypt([]byte("payload"))
	if _, err := b2.Decrypt(enc); err == nil {
		t.Fatal("expected failure decrypting with a different key")
	}
}

func TestNewRejectsBadKey(t *testing.T) {
	if _, err := New("not-base64!!!"); err == nil {
		t.Fatal("expected error on non-base64 key")
	}
	short := base64.StdEncoding.EncodeToString([]byte("tooshort"))
	if _, err := New(short); err == nil {
		t.Fatal("expected error on wrong-length key")
	}
}
