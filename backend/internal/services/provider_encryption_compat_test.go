package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// legacyInlineEncrypt reproduces the pre-#294 inline AES-256-GCM used by the
// model/embedding provider services (fixed 32-byte key via make+copy, random
// nonce prepended, StdEncoding base64). It exists ONLY to generate ciphertext in
// the exact on-the-wire format those services persisted, so the tests below prove
// rows written before #294 remain decryptable through the shared EncryptionService.
func legacyInlineEncrypt(t *testing.T, encryptionKey, plaintext string) string {
	t.Helper()
	key := make([]byte, 32)
	copy(key, []byte(encryptionKey))

	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)
	nonce := make([]byte, gcm.NonceSize())
	_, err = io.ReadFull(rand.Reader, nonce)
	require.NoError(t, err)
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext)
}

// TestProviderServices_DecryptLegacyInlineCiphertext is the #294 compatibility
// gate: secrets encrypted by the removed inline implementation (same 32-byte key)
// must still decrypt after both provider services delegate to the shared
// EncryptionService. If the wire format ever diverged, existing DB rows would
// become undecryptable — this test fails first.
func TestProviderServices_DecryptLegacyInlineCiphertext(t *testing.T) {
	const plaintext = "legacy-provider-value-123"
	// testEncryptionKey (encryption_test.go) is exactly 32 bytes, so the legacy
	// make([]byte,32)+copy is an identity — identical key material to production.
	legacy := legacyInlineEncrypt(t, testEncryptionKey, plaintext)

	t.Run("embedding provider", func(t *testing.T) {
		svc := createTestEmbeddingProviderService(nil)
		got, err := svc.decrypt(legacy)
		require.NoError(t, err)
		assert.Equal(t, plaintext, got)
	})

	t.Run("model provider", func(t *testing.T) {
		svc := createTestModelProviderService(nil)
		got, err := svc.decrypt(legacy)
		require.NoError(t, err)
		assert.Equal(t, plaintext, got)
	})
}
