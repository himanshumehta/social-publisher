package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// EnsureDevAdmin creates (or resets the password of) a local admin account so a
// fresh checkout can be exercised over HTTP without signup or OAuth.
// Callers must only invoke it in development (APP_ENV=development).
func EnsureDevAdmin(ctx context.Context, pool *pgxpool.Pool, email, password string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || password == "" {
		return fmt.Errorf("dev admin email and password are required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash dev admin password: %w", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO users (email, name, password_hash, role, is_active, is_onboarded)
		VALUES ($1, 'Dev Admin', $2, 'admin', TRUE, TRUE)
		ON CONFLICT (email) DO UPDATE
		SET password_hash = EXCLUDED.password_hash,
			role = 'admin',
			is_active = TRUE,
			updated_at = NOW()`,
		email, string(hash),
	)
	if err != nil {
		return fmt.Errorf("upsert dev admin: %w", err)
	}
	return nil
}
