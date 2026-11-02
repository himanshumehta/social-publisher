package service

import (
	"crypto/rand"
	"encoding/hex"
	"testing"
)

// validHexKey generates a random 32-byte key and returns it as a 64-character hex string.
func validHexKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate random key: %v", err)
	}
	return hex.EncodeToString(key)
}

func TestNewTokenCrypto_EmptyKey(t *testing.T) {
	tc, err := NewTokenCrypto("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if tc != nil {
		t.Fatal("expected nil TokenCrypto for empty key")
	}
}

func TestNewTokenCrypto_ValidKey(t *testing.T) {
	hexKey := validHexKey(t)
	tc, err := NewTokenCrypto(hexKey)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if tc == nil {
		t.Fatal("expected non-nil TokenCrypto for valid key")
	}
}

func TestNewTokenCrypto_InvalidHex(t *testing.T) {
	_, err := NewTokenCrypto("not-valid-hex!@#$%^&*()")
	if err == nil {
		t.Fatal("expected error for invalid hex, got nil")
	}
}

func TestNewTokenCrypto_WrongKeyLength(t *testing.T) {
	// 16 bytes (32 hex chars) instead of 32 bytes (64 hex chars)
	shortKey := make([]byte, 16)
	if _, err := rand.Read(shortKey); err != nil {
		t.Fatalf("failed to generate random key: %v", err)
	}
	_, err := NewTokenCrypto(hex.EncodeToString(shortKey))
	if err == nil {
		t.Fatal("expected error for wrong key length, got nil")
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	hexKey := validHexKey(t)
	tc, err := NewTokenCrypto(hexKey)
	if err != nil {
		t.Fatalf("NewTokenCrypto failed: %v", err)
	}

	tests := []string{
		"hello world",
		"a short token",
		"ya29.a0AfH6SMBx-VERY-LONG-OAUTH-TOKEN-1234567890abcdef",
		"unicode: \u00e4\u00f6\u00fc\u00df \xe4\xb8\xad\xe6\x96\x87 \xf0\x9f\x8e\x89",
	}

	for _, plaintext := range tests {
		t.Run(plaintext, func(t *testing.T) {
			encrypted, err := tc.Encrypt(plaintext)
			if err != nil {
				t.Fatalf("Encrypt failed: %v", err)
			}
			if encrypted == plaintext {
				t.Fatal("encrypted text should differ from plaintext")
			}

			decrypted, err := tc.Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt failed: %v", err)
			}
			if decrypted != plaintext {
				t.Errorf("round-trip failed: got %q, want %q", decrypted, plaintext)
			}
		})
	}
}

func TestEncryptDecrypt_EmptyString(t *testing.T) {
	hexKey := validHexKey(t)
	tc, err := NewTokenCrypto(hexKey)
	if err != nil {
		t.Fatalf("NewTokenCrypto failed: %v", err)
	}

	encrypted, err := tc.Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt empty string failed: %v", err)
	}

	decrypted, err := tc.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt empty string failed: %v", err)
	}
	if decrypted != "" {
		t.Errorf("expected empty string, got %q", decrypted)
	}
}

func TestNilTokenCrypto_EncryptIsNoOp(t *testing.T) {
	var tc *TokenCrypto // nil

	got, err := tc.Encrypt("my-secret-token")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "my-secret-token" {
		t.Errorf("nil Encrypt should return plaintext, got %q", got)
	}
}

func TestNilTokenCrypto_DecryptIsNoOp(t *testing.T) {
	var tc *TokenCrypto // nil

	got, err := tc.Decrypt("some-ciphertext")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "some-ciphertext" {
		t.Errorf("nil Decrypt should return ciphertext, got %q", got)
	}
}

func TestDecrypt_InvalidBase64(t *testing.T) {
	hexKey := validHexKey(t)
	tc, err := NewTokenCrypto(hexKey)
	if err != nil {
		t.Fatalf("NewTokenCrypto failed: %v", err)
	}

	_, err = tc.Decrypt("%%%not-base64%%%")
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	key1 := validHexKey(t)
	key2 := validHexKey(t)

	tc1, err := NewTokenCrypto(key1)
	if err != nil {
		t.Fatalf("NewTokenCrypto(key1) failed: %v", err)
	}
	tc2, err := NewTokenCrypto(key2)
	if err != nil {
		t.Fatalf("NewTokenCrypto(key2) failed: %v", err)
	}

	encrypted, err := tc1.Encrypt("secret data")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = tc2.Decrypt(encrypted)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong key, got nil")
	}
}

func TestEncrypt_ProducesDifferentCiphertext(t *testing.T) {
	hexKey := validHexKey(t)
	tc, err := NewTokenCrypto(hexKey)
	if err != nil {
		t.Fatalf("NewTokenCrypto failed: %v", err)
	}

	plaintext := "same-input-every-time"
	enc1, err := tc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("first Encrypt failed: %v", err)
	}
	enc2, err := tc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("second Encrypt failed: %v", err)
	}

	if enc1 == enc2 {
		t.Error("two encryptions of the same plaintext should produce different ciphertext (random nonce)")
	}

	// Both should still decrypt to the same value
	dec1, _ := tc.Decrypt(enc1)
	dec2, _ := tc.Decrypt(enc2)
	if dec1 != plaintext || dec2 != plaintext {
		t.Error("both ciphertexts should decrypt to the original plaintext")
	}
}

func TestTokenCrypto_KeyRotation(t *testing.T) {
	oldKey, newKey := validHexKey(t), validHexKey(t)
	before, _ := NewTokenCrypto(oldKey)
	sealedWithOld, err := before.Encrypt("refresh-token-value")
	if err != nil {
		t.Fatal(err)
	}

	// New key without the old one cannot read existing tokens...
	onlyNew, _ := NewTokenCrypto(newKey)
	if _, err := onlyNew.Decrypt(sealedWithOld); err == nil {
		t.Fatal("decrypted with the wrong key")
	}
	// ...but with the old key listed as previous it can, and new writes use the new key.
	rotated, err := NewTokenCrypto(newKey, oldKey)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := rotated.Decrypt(sealedWithOld); err != nil || got != "refresh-token-value" {
		t.Fatalf("rotated Decrypt = %q, %v", got, err)
	}
	resealed, _ := rotated.Encrypt("refresh-token-value")
	if _, err := before.Decrypt(resealed); err == nil {
		t.Fatal("new ciphertext was sealed with the previous key")
	}
	if got, err := onlyNew.Decrypt(resealed); err != nil || got != "refresh-token-value" {
		t.Fatalf("after rotation the previous key is no longer needed: %q, %v", got, err)
	}
}

func TestNewTokenCrypto_RejectsBadPreviousKey(t *testing.T) {
	if _, err := NewTokenCrypto(validHexKey(t), "not-hex"); err == nil {
		t.Fatal("invalid previous key accepted")
	}
	if _, err := NewTokenCrypto("", validHexKey(t)); err == nil {
		t.Fatal("previous key without a current key accepted")
	}
}

func TestTokenCrypto_SealOpen(t *testing.T) {
	tc, _ := NewTokenCrypto(validHexKey(t))
	stored, encrypted, err := tc.Seal("access")
	if err != nil || !encrypted || stored == "access" {
		t.Fatalf("Seal = %q, %v, %v", stored, encrypted, err)
	}
	if got, err := tc.Open(stored, true); err != nil || got != "access" {
		t.Fatalf("Open = %q, %v", got, err)
	}
	// Legacy plaintext rows (token_encrypted = false) are passed through.
	if got, _ := tc.Open("plain", false); got != "plain" {
		t.Fatalf("Open plaintext = %q", got)
	}

	var none *TokenCrypto
	stored, encrypted, _ = none.Seal("access")
	if encrypted || stored != "access" {
		t.Fatal("without a key Seal must report encrypted=false so the flag stays truthful")
	}
	if _, err := none.Open("c2VhbGVk", true); err != ErrTokenKeyMissing {
		t.Fatalf("Open of an encrypted value without a key: %v, want ErrTokenKeyMissing", err)
	}
}
