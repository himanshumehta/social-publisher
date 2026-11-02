package model

import (
	"time"

	"github.com/google/uuid"
)

type Category struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	Description  string    `json:"description,omitempty"`
	IconURL      string    `json:"icon_url,omitempty"`
	IsActive     bool      `json:"is_active"`
	DisplayOrder int       `json:"display_order"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Channel struct {
	ID               uuid.UUID  `json:"id"`
	CategoryID       *uuid.UUID `json:"category_id,omitempty"`
	UserID           *uuid.UUID `json:"user_id,omitempty"`
	ChannelType      string     `json:"channel_type"`
	Platform         string     `json:"platform"`
	ChannelName      string     `json:"channel_name"`
	ChannelID        string     `json:"channel_id,omitempty"`
	ChannelURL       string     `json:"channel_url,omitempty"`
	AccessToken      string     `json:"-"`
	RefreshToken     string     `json:"-"`
	TokenExpiresAt   *time.Time `json:"token_expires_at,omitempty"`
	OAuthProvider    string     `json:"oauth_provider,omitempty"`
	ConnectionStatus string     `json:"connection_status"`
	Scopes           []string   `json:"scopes,omitempty"`
	IsActive         bool       `json:"is_active"`
	Metadata         []byte     `json:"metadata,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}
