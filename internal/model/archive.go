package model

import (
	"time"

	"github.com/google/uuid"
)

type ArchiveTaskStatus string

const (
	ArchiveTaskPending    ArchiveTaskStatus = "pending"
	ArchiveTaskInProgress ArchiveTaskStatus = "in_progress"
	ArchiveTaskCompleted  ArchiveTaskStatus = "completed"
	ArchiveTaskFailed     ArchiveTaskStatus = "failed"
)

type ArchiveTask struct {
	ID             uuid.UUID         `json:"id"`
	ProjectID      uuid.UUID         `json:"project_id"`
	InitiatedBy    uuid.UUID         `json:"initiated_by"`
	Status         ArchiveTaskStatus `json:"status"`
	TotalFiles     int               `json:"total_files"`
	FilesProcessed int               `json:"files_processed"`
	TotalBytes     int64             `json:"total_bytes"`
	BytesProcessed int64             `json:"bytes_processed"`
	ErrorMessage   *string           `json:"error_message,omitempty"`
	StartedAt      *time.Time        `json:"started_at,omitempty"`
	CompletedAt    *time.Time        `json:"completed_at,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}
