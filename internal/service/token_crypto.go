package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
)

// TokenCrypto provides AES-256-GCM encryption for OAuth tokens stored in the database.
type TokenCrypto struct {
	key []byte // 32 bytes for AES-256
}

// NewTokenCrypto creates a TokenCrypto from a hex-encoded 32-byte key.
// If hexKey is empty, returns nil (encryption disabled — dev mode).
func NewTokenCrypto(hexKey string) (*TokenCrypto, error) {
	if hexKey == "" {
		return nil, nil
	}
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("decode token encryption key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("token encryption key must be 32 bytes (64 hex chars), got %d bytes", len(key))
	}
	return &TokenCrypto{key: key}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM and returns a base64-encoded string.
func (tc *TokenCrypto) Encrypt(plaintext string) (string, error) {
	if tc == nil {
		return plaintext, nil // no-op if encryption disabled
	}

	block, err := aes.NewCipher(tc.key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	// Seal appends the encrypted data to the nonce
	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a base64-encoded AES-256-GCM ciphertext.
func (tc *TokenCrypto) Decrypt(ciphertext string) (string, error) {
	if tc == nil {
		return ciphertext, nil // no-op if encryption disabled
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	block, err := aes.NewCipher(tc.key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, encrypted := data[:nonceSize], data[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}
