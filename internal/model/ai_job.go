package model

import (
	"time"

	"github.com/google/uuid"
)

type AIJobType string

const (
	AIJobAutoClip     AIJobType = "auto_clip"
	AIJobAutoSubtitle AIJobType = "auto_subtitle"
	AIJobAIBrief      AIJobType = "ai_brief"
)

type AIJobStatus string

const (
	AIJobPending    AIJobStatus = "pending"
	AIJobProcessing AIJobStatus = "processing"
	AIJobCompleted  AIJobStatus = "completed"
	AIJobFailed     AIJobStatus = "failed"
)

type AIJob struct {
	ID                  uuid.UUID   `json:"id"`
	ProjectID           uuid.UUID   `json:"project_id"`
	VideoID             *uuid.UUID  `json:"video_id,omitempty"`
	CreatedBy           uuid.UUID   `json:"created_by"`
	JobType             AIJobType   `json:"job_type"`
	Provider            string      `json:"provider"`
	Config              []byte      `json:"config"`
	Status              AIJobStatus `json:"status"`
	Result              []byte      `json:"result,omitempty"`
	ErrorMessage        *string     `json:"error_message,omitempty"`
	CreditsUsed         int         `json:"credits_used"`
	ProcessingStartedAt *time.Time  `json:"processing_started_at,omitempty"`
	CompletedAt         *time.Time  `json:"completed_at,omitempty"`
	CreatedAt           time.Time   `json:"created_at"`
	UpdatedAt           time.Time   `json:"updated_at"`
}

type AIAsset struct {
	ID         uuid.UUID `json:"id"`
	AIJobID    uuid.UUID `json:"ai_job_id"`
	ProjectID  uuid.UUID `json:"project_id"`
	AssetType  string    `json:"asset_type"`
	StorageKey string    `json:"storage_key,omitempty"`
	Content    []byte    `json:"content,omitempty"`
	Metadata   []byte    `json:"metadata"`
	CreatedAt  time.Time `json:"created_at"`
}

type AICredits struct {
	UserID         uuid.UUID `json:"user_id"`
	Balance        int       `json:"balance"`
	TotalPurchased int       `json:"total_purchased"`
	TotalUsed      int       `json:"total_used"`
	UpdatedAt      time.Time `json:"updated_at"`
}
