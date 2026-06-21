package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testEncryptionKey is a valid 32-byte AES-256 key for tests.
const testEncryptionKey = "12345678901234567890123456789012"

func TestNewEncryptionService(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{
			name:    "valid 32 byte key",
			key:     testEncryptionKey,
			wantErr: false,
		},
		{
			name:    "short key is rejected (no padding)",
			key:     "shortkey",
			wantErr: true,
		},
		{
			name:    "long key is rejected (no truncation)",
			key:     strings.Repeat("a", 100),
			wantErr: true,
		},
		{
			name:    "empty key fails",
			key:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := NewEncryptionService(tt.key)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, svc)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, svc)
				assert.Equal(t, 32, len(svc.key))
			}
		})
	}
}

func TestEncryptionService_Encrypt(t *testing.T) {
	svc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	tests := []struct {
		name      string
		plaintext string
		wantErr   bool
	}{
		{
			name:      "encrypt simple text",
			plaintext: "my-api-key-12345",
			wantErr:   false,
		},
		{
			name:      "encrypt long text",
			plaintext: strings.Repeat("a", 1000),
			wantErr:   false,
		},
		{
			name:      "empty plaintext fails",
			plaintext: "",
			wantErr:   true,
		},
		{
			name:      "special characters",
			plaintext: "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, err := svc.Encrypt(tt.plaintext)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, ciphertext)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, ciphertext)
				// Verify it's base64 encoded
				assert.NotContains(t, ciphertext, tt.plaintext)
			}
		})
	}
}

func TestEncryptionService_Decrypt(t *testing.T) {
	svc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	// First encrypt some data
	plaintext := "my-secret-api-key"
	ciphertext, err := svc.Encrypt(plaintext)
	require.NoError(t, err)

	tests := []struct {
		name       string
		ciphertext string
		wantText   string
		wantErr    bool
	}{
		{
			name:       "decrypt valid ciphertext",
			ciphertext: ciphertext,
			wantText:   plaintext,
			wantErr:    false,
		},
		{
			name:       "empty ciphertext fails",
			ciphertext: "",
			wantText:   "",
			wantErr:    true,
		},
		{
			name:       "invalid base64 fails",
			ciphertext: "not-valid-base64!@#$",
			wantText:   "",
			wantErr:    true,
		},
		{
			name:       "tampered ciphertext fails",
			ciphertext: "VGhpcyBpcyBub3QgYSB2YWxpZCBjaXBoZXJ0ZXh0",
			wantText:   "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := svc.Decrypt(tt.ciphertext)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantText, result)
			}
		})
	}
}

func TestEncryptionService_EncryptDecryptRoundTrip(t *testing.T) {
	svc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	testCases := []string{
		"simple-api-key",
		"Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
		strings.Repeat("x", 5000),
		"special!@#$%^&*()_+-=[]{}|;:,.<>?/~`",
	}

	for _, plaintext := range testCases {
		t.Run(plaintext[:min(20, len(plaintext))], func(t *testing.T) {
			// Encrypt
			encrypted, err := svc.Encrypt(plaintext)
			require.NoError(t, err)
			require.NotEmpty(t, encrypted)

			// Decrypt
			decrypted, err := svc.Decrypt(encrypted)
			require.NoError(t, err)
			assert.Equal(t, plaintext, decrypted)
		})
	}
}

func TestEncryptionService_DifferentKeys(t *testing.T) {
	// Different services with different keys
	svc1, err := NewEncryptionService("key1-123456789012345678901234567")
	require.NoError(t, err)

	svc2, err := NewEncryptionService("key2-123456789012345678901234567")
	require.NoError(t, err)

	plaintext := "secret-data"

	// Encrypt with first service
	encrypted, err := svc1.Encrypt(plaintext)
	require.NoError(t, err)

	// Try to decrypt with second service (should fail)
	_, err = svc2.Decrypt(encrypted)
	assert.Error(t, err, "decryption with different key should fail")
}

func TestEncryptionService_RandomNonce(t *testing.T) {
	svc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	plaintext := "same-plaintext"

	// Encrypt same text multiple times
	encrypted1, err := svc.Encrypt(plaintext)
	require.NoError(t, err)

	encrypted2, err := svc.Encrypt(plaintext)
	require.NoError(t, err)

	// The encrypted values should be different (due to random nonce)
	assert.NotEqual(t, encrypted1, encrypted2, "encrypted values should differ due to random nonce")

	// But both should decrypt to same plaintext
	decrypted1, err := svc.Decrypt(encrypted1)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted1)

	decrypted2, err := svc.Decrypt(encrypted2)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted2)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
