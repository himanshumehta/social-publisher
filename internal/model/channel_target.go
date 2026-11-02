package model

import (
	"time"

	"github.com/google/uuid"
)

type ChannelTarget struct {
	ID                    uuid.UUID  `json:"id"`
	ProjectID             uuid.UUID  `json:"project_id"`
	VideoID               *uuid.UUID `json:"video_id,omitempty"`
	ChannelID             uuid.UUID  `json:"channel_id"`
	ScheduledAt           *time.Time `json:"scheduled_at,omitempty"`
	CreatedBy             uuid.UUID  `json:"created_by"`
	CreatedAt             time.Time  `json:"created_at"`
	PerChannelTitle       string     `json:"per_channel_title,omitempty"`
	PerChannelDescription string     `json:"per_channel_description,omitempty"`
}

type ChannelTargetWithChannel struct {
	ChannelTarget
	ChannelName   string `json:"channel_name"`
	Platform      string `json:"platform"`
	CreatedByName string `json:"created_by_name"`
}
