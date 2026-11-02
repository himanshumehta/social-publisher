package model

import (
	"time"

	"github.com/google/uuid"
)

type SmartClipJobStatus string

const (
	SmartClipJobPending    SmartClipJobStatus = "pending"
	SmartClipJobProcessing SmartClipJobStatus = "processing"
	SmartClipJobCompleted  SmartClipJobStatus = "completed"
	SmartClipJobFailed     SmartClipJobStatus = "failed"
	SmartClipJobCancelled  SmartClipJobStatus = "cancelled"
)

type MomentType string

const (
	MomentHook        MomentType = "hook"
	MomentClimax      MomentType = "climax"
	MomentReveal      MomentType = "reveal"
	MomentReaction    MomentType = "reaction"
	MomentTutorial    MomentType = "tutorial_step"
	MomentComparison  MomentType = "comparison"
	MomentBeforeAfter MomentType = "before_after"
	MomentHighlight   MomentType = "highlight"
	MomentQuote       MomentType = "quote"
	MomentCTA         MomentType = "cta"
)

type SmartClipJob struct {
	ID                  uuid.UUID          `json:"id"`
	ProjectID           uuid.UUID          `json:"project_id"`
	VideoID             uuid.UUID          `json:"video_id"`
	CreatedBy           uuid.UUID          `json:"created_by"`
	Status              SmartClipJobStatus `json:"status"`
	Provider            string             `json:"provider"`
	ClipCount           int                `json:"clip_count"`
	MinDurationSeconds  int                `json:"min_duration_seconds"`
	MaxDurationSeconds  int                `json:"max_duration_seconds"`
	TargetAspectRatio   string             `json:"target_aspect_ratio"`
	AutoCaptions        bool               `json:"auto_captions"`
	CaptionLanguage     string             `json:"caption_language"`
	SmartCrop           bool               `json:"smart_crop"`
	DetectHooks         bool               `json:"detect_hooks"`
	Config              []byte             `json:"config,omitempty"`
	CreditsUsed         int                `json:"credits_used"`
	Progress            int                `json:"progress"`
	ErrorMessage        *string            `json:"error_message,omitempty"`
	ProcessingStartedAt *time.Time         `json:"processing_started_at,omitempty"`
	CompletedAt         *time.Time         `json:"completed_at,omitempty"`
	CreatedAt           time.Time          `json:"created_at"`
	UpdatedAt           time.Time          `json:"updated_at"`
}

type SmartClip struct {
	ID                   uuid.UUID `json:"id"`
	JobID                uuid.UUID `json:"job_id"`
	Title                *string   `json:"title,omitempty"`
	StartTimeMs          int       `json:"start_time_ms"`
	EndTimeMs            int       `json:"end_time_ms"`
	DurationMs           int       `json:"duration_ms"`
	HookScore            int       `json:"hook_score"`
	ViralityScore        int       `json:"virality_score"`
	EngagementPrediction float32   `json:"engagement_prediction"`
	AspectRatio          string    `json:"aspect_ratio"`
	HasCaptions          bool      `json:"has_captions"`
	CaptionData          []byte    `json:"caption_data,omitempty"`
	VideoURL             *string   `json:"video_url,omitempty"`
	ThumbnailURL         *string   `json:"thumbnail_url,omitempty"`
	Transcript           *string   `json:"transcript,omitempty"`
	HookText             *string   `json:"hook_text,omitempty"`
	Tags                 []string  `json:"tags"`
	DetectedMoments      []byte    `json:"detected_moments,omitempty"`
	Status               string    `json:"status"`
	Rank                 int       `json:"rank"`
	FileSizeBytes        *int64    `json:"file_size_bytes,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
}

type ClipMoment struct {
	ID          uuid.UUID  `json:"id"`
	JobID       uuid.UUID  `json:"job_id"`
	MomentType  MomentType `json:"moment_type"`
	StartTimeMs int        `json:"start_time_ms"`
	EndTimeMs   int        `json:"end_time_ms"`
	Confidence  float32    `json:"confidence"`
	Description *string    `json:"description,omitempty"`
	HookScore   int        `json:"hook_score"`
	Metadata    []byte     `json:"metadata,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type CreateSmartClipJobRequest struct {
	ProjectID          uuid.UUID `json:"project_id" validate:"required"`
	VideoID            uuid.UUID `json:"video_id" validate:"required"`
	ClipCount          int       `json:"clip_count"`
	MinDurationSeconds int       `json:"min_duration_seconds"`
	MaxDurationSeconds int       `json:"max_duration_seconds"`
	TargetAspectRatio  string    `json:"target_aspect_ratio"`
	AutoCaptions       *bool     `json:"auto_captions,omitempty"`
	CaptionLanguage    string    `json:"caption_language"`
	SmartCrop          *bool     `json:"smart_crop,omitempty"`
	DetectHooks        *bool     `json:"detect_hooks,omitempty"`
}

// HookAnalysis represents analysis of a potential viral hook in the video
type HookAnalysis struct {
	Text       string  `json:"text"`
	StartMs    int     `json:"start_ms"`
	EndMs      int     `json:"end_ms"`
	Score      int     `json:"score"`
	Category   string  `json:"category"` // question, statement, statistic, controversy
	Confidence float32 `json:"confidence"`
}

// ClipScoringInput contains data needed to score a potential clip
type ClipScoringInput struct {
	DurationMs     int      `json:"duration_ms"`
	HasHook        bool     `json:"has_hook"`
	HookScore      int      `json:"hook_score"`
	HasCTA         bool     `json:"has_cta"`
	WordCount      int      `json:"word_count"`
	TopicRelevance float32  `json:"topic_relevance"`
	PacingScore    float32  `json:"pacing_score"`
	Tags           []string `json:"tags"`
}
