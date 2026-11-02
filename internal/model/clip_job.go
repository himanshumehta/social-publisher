package model

import (
	"time"

	"github.com/google/uuid"
)

type ClipJobStatus string

const (
	ClipJobPending    ClipJobStatus = "pending"
	ClipJobProcessing ClipJobStatus = "processing"
	ClipJobCompleted  ClipJobStatus = "completed"
	ClipJobFailed     ClipJobStatus = "failed"
)

type ClipJob struct {
	ID            uuid.UUID     `json:"id"`
	ProjectID     uuid.UUID     `json:"project_id"`
	SourceVideoID uuid.UUID     `json:"source_video_id"`
	Status        ClipJobStatus `json:"status"`
	ClipCount     int           `json:"clip_count"`
	Provider      string        `json:"provider"`
	Config        []byte        `json:"config"`
	ErrorMessage  *string       `json:"error_message,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	StartedAt     *time.Time    `json:"started_at,omitempty"`
	CompletedAt   *time.Time    `json:"completed_at,omitempty"`
}

type ClipJobWithClips struct {
	ClipJob
	Clips []Video `json:"clips"`
}
