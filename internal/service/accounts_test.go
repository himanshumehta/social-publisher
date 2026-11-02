package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/himanshumehta/social-publisher/internal/config"
)

func TestHashPassword_UsesConfiguredCost(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil || cost != BcryptCost {
		t.Fatalf("cost = %d (%v), want %d", cost, err, BcryptCost)
	}
	if _, err := HashPassword(strings.Repeat("x", 73)); err == nil {
		t.Error("passwords over bcrypt's 72-byte limit must be rejected, not truncated")
	}
}

func TestCheckPassword(t *testing.T) {
	hash, _ := HashPassword("s3cret-password")
	if !checkPassword(&hash, "s3cret-password") {
		t.Error("correct password rejected")
	}
	if checkPassword(&hash, "wrong") {
		t.Error("wrong password accepted")
	}
	empty := ""
	if checkPassword(nil, "x") || checkPassword(&empty, "") {
		t.Error("missing hash must never match")
	}
}

func TestNewAPIKey_Format(t *testing.T) {
	a, prefix, err := newAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	b, _, _ := newAPIKey()
	if a == b {
		t.Fatal("keys repeat")
	}
	if !strings.HasPrefix(a, apiKeyPrefix) || len(a) != len(apiKeyPrefix)+43 {
		t.Errorf("unexpected key shape %q", a[:6])
	}
	if !strings.HasPrefix(a, prefix) || len(prefix) >= len(a)/2 {
		t.Errorf("display prefix %q leaks too much of the key", prefix)
	}
	if got := HashAPIKey("abc"); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Errorf("HashAPIKey(abc) = %s", got)
	}
}

// DB-backed: account provisioning, API key round trip and login failures.
func TestAccountsAndLogin(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()
	ctx := context.Background()
	email := fmt.Sprintf("acct-%s@example.com", uuid.New().String()[:8])
	defer pool.Exec(ctx, `DELETE FROM users WHERE email = $1`, email)

	if _, err := CreateUser(ctx, pool, email, "", "admin", "short"); err == nil {
		t.Error("short password accepted")
	}
	if _, err := CreateUser(ctx, pool, email, "", "root", "long-enough-password"); err == nil {
		t.Error("unknown role accepted")
	}
	userID, err := CreateUser(ctx, pool, email, "", "creator", "long-enough-password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	cfg := &config.Config{JWT: config.JWTConfig{Secret: strings.Repeat("k", 40), AccessTokenTTL: time.Minute, RefreshTokenTTL: time.Hour}}
	auth := NewAuthService(cfg, pool)
	if _, _, err := auth.Login(ctx, LoginRequest{Email: strings.ToUpper(email), Password: "long-enough-password"}); err != nil {
		t.Fatalf("valid login failed: %v", err)
	}
	for name, req := range map[string]LoginRequest{
		"unknown email":  {Email: "nobody-" + email, Password: "long-enough-password"},
		"wrong password": {Email: email, Password: "wrong-password-123"},
		"empty":          {},
	} {
		if _, _, err := auth.Login(ctx, req); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("%s: err = %v, want ErrInvalidCredentials", name, err)
		}
	}

	if _, err := IssueAPIKey(ctx, pool, email, "ci", []string{"admin:all"}, 0); err == nil {
		t.Error("unknown scope accepted")
	}
	raw, err := IssueAPIKey(ctx, pool, email, "ci", []string{"posts:create"}, time.Hour)
	if err != nil {
		t.Fatalf("IssueAPIKey: %v", err)
	}
	var stored string
	if err := pool.QueryRow(ctx, `SELECT key_hash FROM api_keys WHERE user_id = $1`, userID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == raw || stored != HashAPIKey(raw) {
		t.Fatal("api_keys must store the SHA-256 digest, never the key")
	}
	keys := NewAPIKeyService(pool, nil)
	if k, err := keys.ValidateKey(ctx, HashAPIKey(raw)); err != nil || k.UserID != userID {
		t.Fatalf("ValidateKey: %v", err)
	}

	// Deactivating the owner disables both password login and its API keys.
	if _, err := pool.Exec(ctx, `UPDATE users SET is_active = FALSE WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Login(ctx, LoginRequest{Email: email, Password: "long-enough-password"}); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("inactive login: err = %v", err)
	}
	if _, err := keys.ValidateKey(ctx, HashAPIKey(raw)); err == nil {
		t.Error("API key of a deactivated user still validates")
	}
}
