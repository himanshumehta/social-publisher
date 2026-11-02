package model

import (
	"time"

	"github.com/google/uuid"
)

// APIKey represents an API key for external service access.
type APIKey struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	KeyHash    string     `json:"-"`
	KeyPrefix  string     `json:"key_prefix"`
	UserID     uuid.UUID  `json:"user_id"`
	Scopes     []string   `json:"scopes"`
	IsActive   bool       `json:"is_active"`
	RateLimit  int        `json:"rate_limit"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
