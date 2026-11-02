package service

import (
	"testing"
)

func TestGenerateRandomString(t *testing.T) {
	s1, err := generateRandomString(32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s1) != 64 { // hex encoding doubles the length
		t.Errorf("expected length 64, got %d", len(s1))
	}

	s2, err := generateRandomString(32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if s1 == s2 {
		t.Error("two random strings should not be equal")
	}
}

func TestGenerateRandomString_DifferentLengths(t *testing.T) {
	tests := []struct {
		n           int
		expectedLen int
	}{
		{4, 8},
		{16, 32},
		{64, 128},
	}
	for _, tt := range tests {
		s, err := generateRandomString(tt.n)
		if err != nil {
			t.Fatalf("unexpected error for n=%d: %v", tt.n, err)
		}
		if len(s) != tt.expectedLen {
			t.Errorf("n=%d: expected length %d, got %d", tt.n, tt.expectedLen, len(s))
		}
	}
}

func TestSha256Hash(t *testing.T) {
	h1 := sha256Hash("hello")
	h2 := sha256Hash("hello")
	h3 := sha256Hash("world")

	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	if h1 == h3 {
		t.Error("different inputs should produce different hashes")
	}
	if len(h1) != 64 { // SHA-256 hex is 64 chars
		t.Errorf("expected hash length 64, got %d", len(h1))
	}
}

func TestSha256Hash_KnownValue(t *testing.T) {
	// SHA-256 of "test" is known
	h := sha256Hash("test")
	expected := "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	if h != expected {
		t.Errorf("expected %s, got %s", expected, h)
	}
}
