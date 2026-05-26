package model

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/NLLCommunity/heimdallr/config"
)

// TokenCrypto encrypts and decrypts short-lived bearer tokens (OAuth2
// access / refresh tokens) for at-rest storage in the dashboard_sessions
// table. It is deliberately small — AES-256-GCM with a per-message
// random nonce, base64 standard encoding for storage — because the only
// reason for it to exist is that a DB leak shouldn't hand out call-
// anything Discord bearer tokens. It is not a general-purpose crypto
// abstraction.
//
// Construction reads the key once from config (dashboard.token_encryption_key)
// and panics on a missing or wrong-length key, because the only sensible
// response to a misconfigured key on a process that is about to write user
// tokens is to refuse to start.
type TokenCrypto struct {
	aead cipher.AEAD
}

// NewTokenCrypto builds a TokenCrypto from the configured key. Callers
// (web server startup) invoke this during initialization and pass the
// returned instance into anything that touches DashboardSession.AccessToken
// or RefreshToken.
func NewTokenCrypto() (*TokenCrypto, error) {
	key, err := config.DashboardTokenEncryptionKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("token_encryption_key: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("token_encryption_key: %w", err)
	}
	return &TokenCrypto{aead: aead}, nil
}

// Seal returns a base64-encoded, nonce-prefixed ciphertext for plaintext.
// Empty input round-trips to an empty string, so callers that store
// "no token yet" rows don't have to special-case it.
func (c *TokenCrypto) Seal(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("token seal: nonce: %w", err)
	}
	ct := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// Open reverses Seal. An empty input round-trips back to an empty string;
// any other value that fails to decode or authenticate returns an error.
// Callers should treat decryption failure as "this session is unusable,
// force re-login" rather than as a recoverable condition.
func (c *TokenCrypto) Open(sealed string) (string, error) {
	if sealed == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return "", fmt.Errorf("token open: base64: %w", err)
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("token open: ciphertext too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	pt, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("token open: %w", err)
	}
	return string(pt), nil
}
