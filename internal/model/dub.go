package model

import (
	"time"

	"github.com/google/uuid"
)

type DubJobStatus string

const (
	DubJobStatusPending    DubJobStatus = "pending"
	DubJobStatusProcessing DubJobStatus = "processing"
	DubJobStatusCompleted  DubJobStatus = "completed"
	DubJobStatusFailed     DubJobStatus = "failed"
	DubJobStatusCancelled  DubJobStatus = "cancelled"
)

var SupportedDubLanguages = map[string]string{
	"hi": "Hindi",
	"ta": "Tamil",
	"te": "Telugu",
	"kn": "Kannada",
	"ml": "Malayalam",
	"bn": "Bengali",
	"mr": "Marathi",
	"gu": "Gujarati",
	"pa": "Punjabi",
}

type DubJob struct {
	ID                      uuid.UUID    `json:"id"`
	ProjectID               uuid.UUID    `json:"project_id"`
	VideoID                 uuid.UUID    `json:"video_id"`
	CreatedBy               uuid.UUID    `json:"created_by"`
	SourceLanguage          string       `json:"source_language"`
	TargetLanguage          string       `json:"target_language"`
	Status                  DubJobStatus `json:"status"`
	Provider                string       `json:"provider"`
	Config                  []byte       `json:"config,omitempty"`
	VoiceProfile            *string      `json:"voice_profile,omitempty"`
	PreserveBackgroundAudio bool         `json:"preserve_background_audio"`
	LipSyncEnabled          bool         `json:"lip_sync_enabled"`
	CreditsUsed             int          `json:"credits_used"`
	Progress                int          `json:"progress"`
	ErrorMessage            *string      `json:"error_message,omitempty"`
	Result                  []byte       `json:"result,omitempty"`
	OutputVideoURL          *string      `json:"output_video_url,omitempty"`
	OutputAudioURL          *string      `json:"output_audio_url,omitempty"`
	DurationSeconds         *int         `json:"duration_seconds,omitempty"`
	ProcessingStartedAt     *time.Time   `json:"processing_started_at,omitempty"`
	CompletedAt             *time.Time   `json:"completed_at,omitempty"`
	CreatedAt               time.Time    `json:"created_at"`
	UpdatedAt               time.Time    `json:"updated_at"`
}

type DubOutput struct {
	ID              uuid.UUID `json:"id"`
	DubJobID        uuid.UUID `json:"dub_job_id"`
	Language        string    `json:"language"`
	AudioURL        *string   `json:"audio_url,omitempty"`
	VideoURL        *string   `json:"video_url,omitempty"`
	SubtitleURL     *string   `json:"subtitle_url,omitempty"`
	Transcript      *string   `json:"transcript,omitempty"`
	QualityScore    *float32  `json:"quality_score,omitempty"`
	DurationSeconds *int      `json:"duration_seconds,omitempty"`
	FileSizeBytes   *int64    `json:"file_size_bytes,omitempty"`
	Status          string    `json:"status"`
	ErrorMessage    *string   `json:"error_message,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type CreateDubJobRequest struct {
	ProjectID               uuid.UUID `json:"project_id" validate:"required"`
	VideoID                 uuid.UUID `json:"video_id" validate:"required"`
	TargetLanguage          string    `json:"target_language" validate:"required"`
	SourceLanguage          string    `json:"source_language"`
	VoiceProfile            *string   `json:"voice_profile,omitempty"`
	PreserveBackgroundAudio *bool     `json:"preserve_background_audio,omitempty"`
	LipSyncEnabled          *bool     `json:"lip_sync_enabled,omitempty"`
}

type BatchDubRequest struct {
	ProjectID       uuid.UUID `json:"project_id" validate:"required"`
	VideoID         uuid.UUID `json:"video_id" validate:"required"`
	TargetLanguages []string  `json:"target_languages" validate:"required,min=1"`
	VoiceProfile    *string   `json:"voice_profile,omitempty"`
}

type DubLanguageInfo struct {
	Code string `json:"code"`
	Name string `json:"name"`
}
