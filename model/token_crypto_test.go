package model

import (
	"crypto/aes"
	"crypto/cipher"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withTestAEAD swaps in a fixed cipher.AEAD for the duration of a test
// helper, bypassing the config-driven NewTokenCrypto so the round-trip
// tests don't depend on the viper-backed key.
func newTestTokenCrypto(t *testing.T) *TokenCrypto {
	t.Helper()
	// Use AES-256-GCM via NewTokenCrypto's normal codepath would require
	// setting viper config — instead build the AEAD directly here.
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i) // deterministic, non-secret — test-only
	}
	c, err := newTokenCryptoFromKey(key)
	require.NoError(t, err)
	return c
}

func TestTokenCrypto_RoundTrip(t *testing.T) {
	c := newTestTokenCrypto(t)
	cases := []string{
		"short-access-token",
		"some-refresh-token-with-mixed-content-1234567890",
		// Empty input must round-trip back to empty without error: callers
		// that store "no token yet" rows shouldn't have to special-case it.
		"",
	}
	for _, plain := range cases {
		sealed, err := c.Seal(plain)
		require.NoError(t, err)
		if plain == "" {
			assert.Equal(t, "", sealed, "empty plaintext must seal to empty string")
		} else {
			assert.NotEqual(t, plain, sealed, "ciphertext must not match plaintext")
		}
		opened, err := c.Open(sealed)
		require.NoError(t, err)
		assert.Equal(t, plain, opened)
	}
}

func TestTokenCrypto_UniqueCiphertexts(t *testing.T) {
	// A fresh nonce on every Seal means two encryptions of the same input
	// produce different ciphertexts. This isn't a strict security
	// requirement here (we'd be using deterministic encryption otherwise),
	// but it's an easy regression check that the nonce isn't accidentally
	// zeroed.
	c := newTestTokenCrypto(t)
	a, err := c.Seal("same-input")
	require.NoError(t, err)
	b, err := c.Seal("same-input")
	require.NoError(t, err)
	assert.NotEqual(t, a, b, "ciphertexts must differ across calls (random nonce)")
}

func TestTokenCrypto_OpenTamperedFails(t *testing.T) {
	// Sanity-check GCM authentication: flipping any bit in the ciphertext
	// must cause Open to fail rather than silently return garbage.
	c := newTestTokenCrypto(t)
	sealed, err := c.Seal("authentic-data")
	require.NoError(t, err)
	require.NotEmpty(t, sealed)

	// Tamper by changing the last character. base64 alphabet has plenty
	// of substitutions; pick a deterministic one that's always different.
	tampered := []byte(sealed)
	if tampered[len(tampered)-1] == 'A' {
		tampered[len(tampered)-1] = 'B'
	} else {
		tampered[len(tampered)-1] = 'A'
	}

	_, err = c.Open(string(tampered))
	assert.Error(t, err, "tampered ciphertext must fail authentication")
}

func TestTokenCrypto_OpenGarbageFails(t *testing.T) {
	c := newTestTokenCrypto(t)
	// Not base64.
	_, err := c.Open("!!!not-base64!!!")
	assert.Error(t, err)
	// Valid base64 but too short to contain a nonce.
	_, err = c.Open("YQ==")
	assert.Error(t, err)
}

// newTokenCryptoFromKey is a test seam: it lets tests build a TokenCrypto
// from a hardcoded key without depending on viper config. Kept in the
// same package as TokenCrypto so the unexported field stays unexported.
func newTokenCryptoFromKey(key []byte) (*TokenCrypto, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &TokenCrypto{aead: aead}, nil
}
