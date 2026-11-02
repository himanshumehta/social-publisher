package model

import (
	"time"

	"github.com/google/uuid"
)

type PublishRecord struct {
	ID                uuid.UUID  `json:"id"`
	ProjectID         uuid.UUID  `json:"project_id"`
	VideoID           *uuid.UUID `json:"video_id,omitempty"`
	ChannelID         uuid.UUID  `json:"channel_id"`
	PlatformContentID string     `json:"platform_content_id,omitempty"`
	PlatformURL       string     `json:"platform_url,omitempty"`
	Title             string     `json:"title,omitempty"`
	Description       string     `json:"description,omitempty"`
	Tags              []string   `json:"tags,omitempty"`
	ThumbnailURL      string     `json:"thumbnail_url,omitempty"`
	Visibility        string     `json:"visibility"`
	Status            string     `json:"status"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	ScheduledAt       *time.Time `json:"scheduled_at,omitempty"`
	PublishedAt       *time.Time `json:"published_at,omitempty"`
	Metadata          []byte     `json:"metadata,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// PublishRecordWithChannel embeds PublishRecord with channel info for list responses.
type PublishRecordWithChannel struct {
	ID                uuid.UUID  `json:"id"`
	ProjectID         uuid.UUID  `json:"project_id"`
	ChannelID         uuid.UUID  `json:"channel_id"`
	ChannelName       string     `json:"channel_name"`
	Platform          string     `json:"platform"`
	PlatformContentID string     `json:"platform_content_id,omitempty"`
	PlatformURL       string     `json:"platform_url,omitempty"`
	Title             string     `json:"title,omitempty"`
	Description       string     `json:"description,omitempty"`
	Visibility        string     `json:"visibility"`
	Status            string     `json:"status"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	PublishedAt       *time.Time `json:"published_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}
