package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// TokenCrypto seals OAuth access and refresh tokens before they are written
// to the channels table, with AES-256-GCM (random 96-bit nonce per value,
// stored as base64(nonce || ciphertext || tag)).
//
// Rotation: Encrypt always uses the current key; Decrypt also accepts any
// previous key (TOKEN_ENCRYPTION_KEY_PREVIOUS). Tokens are re-sealed with the
// current key whenever they are next refreshed or reconnected, after which
// the previous key can be dropped.
type TokenCrypto struct {
	aeads []cipher.AEAD // aeads[0] is the current key
}

// ErrTokenKeyMissing is returned when a stored token is marked encrypted but
// the process has no key to open it.
var ErrTokenKeyMissing = errors.New("token is encrypted but TOKEN_ENCRYPTION_KEY is not set")

// NewTokenCrypto builds a TokenCrypto from hex-encoded 32-byte keys. An empty
// current key returns (nil, nil): encryption disabled, which config.Validate
// only allows in development.
func NewTokenCrypto(current string, previous ...string) (*TokenCrypto, error) {
	if current == "" {
		if len(previous) > 0 {
			return nil, fmt.Errorf("TOKEN_ENCRYPTION_KEY_PREVIOUS is set but TOKEN_ENCRYPTION_KEY is empty")
		}
		return nil, nil
	}
	tc := &TokenCrypto{}
	for i, hexKey := range append([]string{current}, previous...) {
		aead, err := newAEAD(hexKey)
		if err != nil {
			if i == 0 {
				return nil, fmt.Errorf("token encryption key: %w", err)
			}
			return nil, fmt.Errorf("previous token encryption key %d: %w", i, err)
		}
		tc.aeads = append(tc.aeads, aead)
	}
	return tc, nil
}

func newAEAD(hexKey string) (cipher.AEAD, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("decode hex: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("must be 32 bytes (64 hex chars), got %d bytes", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

// Enabled reports whether tokens are encrypted at rest.
func (tc *TokenCrypto) Enabled() bool { return tc != nil }

// Encrypt seals plaintext with the current key and returns base64 text.
// On a nil TokenCrypto (development without a key) it returns plaintext.
func (tc *TokenCrypto) Encrypt(plaintext string) (string, error) {
	if tc == nil {
		return plaintext, nil
	}
	aead := tc.aeads[0]
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	return base64.StdEncoding.EncodeToString(aead.Seal(nonce, nonce, []byte(plaintext), nil)), nil
}

// Decrypt opens a value produced by Encrypt, trying the current key first and
// then each previous key. On a nil TokenCrypto it returns the input unchanged.
func (tc *TokenCrypto) Decrypt(ciphertext string) (string, error) {
	if tc == nil {
		return ciphertext, nil
	}
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}
	for _, aead := range tc.aeads {
		ns := aead.NonceSize()
		if len(data) < ns {
			return "", fmt.Errorf("ciphertext too short")
		}
		if plaintext, err := aead.Open(nil, data[:ns], data[ns:], nil); err == nil {
			return string(plaintext), nil
		}
	}
	return "", fmt.Errorf("decrypt: no configured key opens this token")
}

// Seal prepares a token for storage and reports whether the stored value is
// encrypted, which callers persist in channels.token_encrypted.
func (tc *TokenCrypto) Seal(plaintext string) (stored string, encrypted bool, err error) {
	if plaintext == "" {
		return "", tc.Enabled(), nil
	}
	stored, err = tc.Encrypt(plaintext)
	return stored, tc.Enabled(), err
}

// Open returns the plaintext of a stored token given its token_encrypted flag.
// It fails, rather than passing ciphertext on to a platform API, when the
// value is encrypted and no key is configured.
func (tc *TokenCrypto) Open(stored string, encrypted bool) (string, error) {
	if !encrypted || stored == "" {
		return stored, nil
	}
	if tc == nil {
		return "", ErrTokenKeyMissing
	}
	return tc.Decrypt(stored)
}
