package model

import (
	"time"

	"github.com/google/uuid"
)

type VideoStatus string

const (
	VideoStatusProcessing VideoStatus = "processing"
	VideoStatusReady      VideoStatus = "ready"
	VideoStatusApproved   VideoStatus = "approved"
	VideoStatusRejected   VideoStatus = "rejected"
)

type VideoFormat string

const (
	VideoFormatRaw           VideoFormat = "raw"
	VideoFormatYouTube16x9   VideoFormat = "youtube_16_9"
	VideoFormatInstagramReel VideoFormat = "instagram_reel"
	VideoFormatYouTubeShort  VideoFormat = "youtube_short"
	VideoFormatFacebookFeed  VideoFormat = "facebook_feed"
	VideoFormatFacebookReel  VideoFormat = "facebook_reel"
)

type Video struct {
	ID              uuid.UUID   `json:"id"`
	ProjectID       uuid.UUID   `json:"project_id"`
	UploadedBy      uuid.UUID   `json:"uploaded_by"`
	Format          VideoFormat `json:"format"`
	Version         int         `json:"version"`
	StorageKey      string      `json:"storage_key"`
	StorageType     string      `json:"storage_type"`
	FileName        string      `json:"file_name"`
	FileSizeBytes   int64       `json:"file_size_bytes"`
	MimeType        string      `json:"mime_type"`
	DurationSeconds *int        `json:"duration_seconds,omitempty"`
	Width           *int        `json:"width,omitempty"`
	Height          *int        `json:"height,omitempty"`
	Status          VideoStatus `json:"status"`
	SourceVideoID   *uuid.UUID  `json:"source_video_id,omitempty"`
	ClipJobID       *uuid.UUID  `json:"clip_job_id,omitempty"`
	ViralityScore   *int        `json:"virality_score,omitempty"`
	ThumbnailKey    *string     `json:"thumbnail_key,omitempty"`
	IsCurrent       bool        `json:"is_current"`
	IsDeleted       bool        `json:"-"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

type MultipartUpload struct {
	ID            uuid.UUID `json:"id"`
	UploadID      string    `json:"upload_id"`
	ProjectID     uuid.UUID `json:"project_id"`
	UserID        uuid.UUID `json:"user_id"`
	StorageKey    string    `json:"storage_key"`
	FileName      string    `json:"file_name"`
	FileSizeBytes int64     `json:"file_size_bytes"`
	MimeType      string    `json:"mime_type"`
	TotalParts    int       `json:"total_parts"`
	Format        string    `json:"format"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
