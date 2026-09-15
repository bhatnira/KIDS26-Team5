// Package secretbox provides authenticated symmetric encryption (AES-256-GCM)
// for protecting secrets at rest — e.g. per-user object-storage and LLM
// credentials persisted in Postgres. Ciphertexts are self-describing
// (nonce-prefixed) and authenticated, so tampering is detected on Decrypt.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// Box encrypts and decrypts with a fixed AES-256-GCM key.
type Box struct {
	gcm cipher.AEAD
}

// New builds a Box from a base64-encoded 32-byte (AES-256) key. The key is
// typically supplied via ANTELOPE_SYSTEM_ENCRYPT_KEY.
func New(keyB64 string) (*Box, error) {
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("secretbox: decode key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("secretbox: key must be 32 bytes (AES-256) base64-encoded, got %d bytes", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: new gcm: %w", err)
	}
	return &Box{gcm: gcm}, nil
}

// Encrypt returns base64( nonce || ciphertext+tag ).
func (b *Box) Encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, b.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("secretbox: read nonce: %w", err)
	}
	sealed := b.gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt and verifies authenticity.
func (b *Box) Decrypt(enc string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return nil, fmt.Errorf("secretbox: decode ciphertext: %w", err)
	}
	ns := b.gcm.NonceSize()
	if len(raw) < ns {
		return nil, errors.New("secretbox: ciphertext too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	pt, err := b.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("secretbox: authenticate/decrypt: %w", err)
	}
	return pt, nil
}

// GenerateKey returns a new base64-encoded 32-byte key suitable for New. Useful
// for operators bootstrapping ANTELOPE_SYSTEM_ENCRYPT_KEY.
func GenerateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}
