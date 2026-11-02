package model

import (
	"time"

	"github.com/google/uuid"
)

type RawAssetStatus string

const (
	RawAssetUploading  RawAssetStatus = "uploading"
	RawAssetProcessing RawAssetStatus = "processing"
	RawAssetReady      RawAssetStatus = "ready"
	RawAssetInUse      RawAssetStatus = "in_use"
	RawAssetRejected   RawAssetStatus = "rejected"
	RawAssetSuperseded RawAssetStatus = "superseded"
)

type RawAsset struct {
	ID              uuid.UUID      `json:"id"`
	ProjectID       uuid.UUID      `json:"project_id"`
	UploadedBy      uuid.UUID      `json:"uploaded_by"`
	StorageKey      string         `json:"storage_key"`
	StorageType     string         `json:"storage_type"`
	FileName        string         `json:"file_name"`
	FileSizeBytes   int64          `json:"file_size_bytes"`
	MimeType        string         `json:"mime_type"`
	DurationSeconds *int           `json:"duration_seconds,omitempty"`
	Width           *int           `json:"width,omitempty"`
	Height          *int           `json:"height,omitempty"`
	Status          RawAssetStatus `json:"status"`
	Notes           string         `json:"notes,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}
