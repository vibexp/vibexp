package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// EncryptionService handles encryption and decryption of sensitive data using AES-256-GCM
type EncryptionService struct {
	key []byte
}

// encryptionKeyLength is the required AES-256 key length in bytes.
const encryptionKeyLength = 32

// NewEncryptionService creates a new encryption service with the provided key.
// The key must be exactly 32 bytes (AES-256). It fails closed: keys that are empty
// or the wrong length are rejected rather than silently padded or truncated, which
// would weaken the cipher.
func NewEncryptionService(key string) (*EncryptionService, error) {
	if key == "" {
		return nil, errors.New("encryption key is required")
	}

	keyBytes := []byte(key)
	if len(keyBytes) != encryptionKeyLength {
		return nil, fmt.Errorf(
			"encryption key must be exactly %d bytes, got %d", encryptionKeyLength, len(keyBytes),
		)
	}

	return &EncryptionService{
		key: keyBytes,
	}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM
// Returns base64 encoded ciphertext with nonce prepended
func (s *EncryptionService) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", errors.New("plaintext cannot be empty")
	}

	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate a random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the plaintext
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Encode to base64 for safe storage
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts base64 encoded ciphertext using AES-256-GCM
func (s *EncryptionService) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", errors.New("ciphertext cannot be empty")
	}

	// Decode from base64
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	// Extract nonce and ciphertext
	nonce, cipherBytes := data[:nonceSize], data[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, cipherBytes, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}
