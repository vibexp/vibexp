// Package oauthserver implements VibeXP's embedded OAuth 2.1 Authorization
// Server (issue #31). It wraps ory/fosite for the crypto-sensitive token
// issuance (authorize/token/PKCE/refresh rotation, JWT access tokens) and owns
// the thin MCP-specific layer: dynamic client registration, the federated login
// leg (delegating user authentication to the #29 identity-provider registry), a
// consent step, DB-backed rotating signing keys served via JWKS, and RFC 8414
// authorization-server metadata. Issued access tokens are JWTs audience-bound to
// the configured MCP resource URI (RFC 8707).
package oauthserver

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
)

// seal encrypts plaintext with AES-256-GCM and returns nonce||ciphertext. key
// must be exactly 32 bytes (the app encryption key).
func seal(key, plaintext []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("oauthserver: generate nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// open reverses seal.
func open(key, blob []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(blob) < ns {
		return nil, fmt.Errorf("oauthserver: ciphertext too short")
	}
	nonce, ciphertext := blob[:ns], blob[ns:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("oauthserver: decrypt: %w", err)
	}
	return plaintext, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("oauthserver: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("oauthserver: new gcm: %w", err)
	}
	return gcm, nil
}

// deriveSecret returns a 32-byte key derived from the master key and a
// domain-separation tag, so fosite's HMAC global secret never reuses the raw
// AES key directly.
func deriveSecret(masterKey []byte, tag string) []byte {
	mac := hmac.New(sha256.New, masterKey)
	mac.Write([]byte(tag))
	return mac.Sum(nil)
}
