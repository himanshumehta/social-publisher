package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type APIKeyService struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func NewAPIKeyService(pool *pgxpool.Pool, logger *slog.Logger) *APIKeyService {
	return &APIKeyService{pool: pool, logger: logger}
}

func (s *APIKeyService) ValidateKey(ctx context.Context, keyHash string) (*model.APIKey, error) {
	apiKey := &model.APIKey{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, key_prefix, user_id, scopes, is_active, rate_limit, last_used_at, expires_at
		FROM api_keys
		WHERE key_hash = $1 AND is_active = TRUE`,
		keyHash,
	).Scan(
		&apiKey.ID, &apiKey.Name, &apiKey.KeyPrefix, &apiKey.UserID,
		&apiKey.Scopes, &apiKey.IsActive, &apiKey.RateLimit,
		&apiKey.LastUsedAt, &apiKey.ExpiresAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("invalid or inactive API key")
		}
		return nil, err
	}

	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("API key has expired")
	}

	go func() {
		s.pool.Exec(context.Background(), `
			UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`, apiKey.ID)
	}()

	return apiKey, nil
}
