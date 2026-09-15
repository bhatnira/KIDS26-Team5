package auth

import (
	"errors"
	"strings"
)

// encPrefix marks a provider secret stored as AES-GCM ciphertext. Values lacking
// this prefix are treated as legacy plaintext, so existing rows keep working and
// no data migration is needed; they upgrade to ciphertext the next time the
// provider is saved.
const encPrefix = "enc:v1:"

// encryptSecret returns the provider secret prefixed ciphertext when an
// encryption box is configured, or the plaintext unchanged when it is not
// (durable but unencrypted — matching the storage/LLM at-rest fallback). Empty
// input is returned as-is so a blank field is never turned into ciphertext.
func (s *authService) encryptSecret(plain string) (string, error) {
	if plain == "" || s.box == nil {
		return plain, nil
	}
	enc, err := s.box.Encrypt([]byte(plain))
	if err != nil {
		return "", err
	}
	return encPrefix + enc, nil
}

// decryptSecret reverses encryptSecret. A value without encPrefix is legacy
// plaintext (or was written while no key was configured) and is returned as-is.
// A prefixed value with no box configured is an error rather than a silent
// corruption.
func (s *authService) decryptSecret(stored string) (string, error) {
	if !strings.HasPrefix(stored, encPrefix) {
		return stored, nil
	}
	if s.box == nil {
		return "", errors.New("auth: provider secret is encrypted but no encryption key is configured")
	}
	pt, err := s.box.Decrypt(strings.TrimPrefix(stored, encPrefix))
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
