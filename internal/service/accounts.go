package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Accounts are provisioned by an operator through cmd/admin; there is no
// public signup endpoint.

// ValidRoles are the user roles cmd/admin may assign.
var ValidRoles = map[string]bool{"creator": true, "editor": true, "admin": true, "super_admin": true}

// ValidAPIKeyScopes are the scopes the external API checks.
var ValidAPIKeyScopes = map[string]bool{"posts:create": true, "posts:read": true}

// apiKeyPrefix makes keys recognisable in secret scanners and logs.
const apiKeyPrefix = "sp_"

// CreateUser inserts an active account with a bcrypt password hash.
func CreateUser(ctx context.Context, pool *pgxpool.Pool, email, name, role, password string) (uuid.UUID, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || !strings.Contains(email, "@") {
		return uuid.Nil, fmt.Errorf("a valid email is required")
	}
	if !ValidRoles[role] {
		return uuid.Nil, fmt.Errorf("invalid role %q", role)
	}
	if len(password) < MinPasswordLength {
		return uuid.Nil, fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	}
	if name == "" {
		name = email
	}
	hash, err := HashPassword(password)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO users (email, name, password_hash, role, is_active, is_onboarded)
		VALUES ($1, $2, $3, $4::user_role, TRUE, TRUE)
		RETURNING id`,
		email, name, hash, role,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create user: %w", err)
	}
	return id, nil
}

// HashAPIKey is the one-way digest stored for an API key. Keys carry 256 bits
// of randomness, so a fast hash is sufficient (no need for bcrypt).
func HashAPIKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// newAPIKey returns a random key and the short prefix stored for display.
func newAPIKey() (raw, prefix string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate api key: %w", err)
	}
	raw = apiKeyPrefix + base64.RawURLEncoding.EncodeToString(b)
	return raw, raw[:len(apiKeyPrefix)+6], nil
}

// IssueAPIKey creates an API key for the user with the given email. The raw
// key is returned once; only its SHA-256 digest is stored.
func IssueAPIKey(ctx context.Context, pool *pgxpool.Pool, userEmail, name string, scopes []string, ttl time.Duration) (string, error) {
	if name == "" {
		return "", fmt.Errorf("a key name is required")
	}
	if len(scopes) == 0 {
		return "", fmt.Errorf("at least one scope is required")
	}
	for _, s := range scopes {
		if !ValidAPIKeyScopes[s] {
			return "", fmt.Errorf("invalid scope %q", s)
		}
	}
	var userID uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM users WHERE email = $1 AND is_active = TRUE`,
		strings.TrimSpace(strings.ToLower(userEmail)),
	).Scan(&userID); err != nil {
		return "", fmt.Errorf("no active user %q: %w", userEmail, err)
	}

	raw, prefix, err := newAPIKey()
	if err != nil {
		return "", err
	}
	var expiresAt *time.Time
	if ttl > 0 {
		t := time.Now().Add(ttl)
		expiresAt = &t
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO api_keys (name, key_hash, key_prefix, user_id, scopes, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		name, HashAPIKey(raw), prefix, userID, scopes, expiresAt,
	); err != nil {
		return "", fmt.Errorf("store api key: %w", err)
	}
	return raw, nil
}
