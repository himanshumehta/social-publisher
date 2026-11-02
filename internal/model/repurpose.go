package model

import (
	"time"

	"github.com/google/uuid"
)

// RepurposeStrategy defines the content transformation approach.
type RepurposeStrategy string

const (
	StrategyLongToShort    RepurposeStrategy = "long_to_short"
	StrategyBlogToThread   RepurposeStrategy = "blog_to_thread"
	StrategyVideoToCaption RepurposeStrategy = "video_to_caption"
	StrategyPodcastToBlog  RepurposeStrategy = "podcast_to_blog"
	StrategyCrossPlatform  RepurposeStrategy = "cross_platform"
)

// ValidStrategies lists all allowed repurpose strategies.
var ValidStrategies = map[RepurposeStrategy]bool{
	StrategyLongToShort:    true,
	StrategyBlogToThread:   true,
	StrategyVideoToCaption: true,
	StrategyPodcastToBlog:  true,
	StrategyCrossPlatform:  true,
}

// RepurposeJobStatus mirrors the generic job status values.
type RepurposeJobStatus string

const (
	RepurposeStatusPending    RepurposeJobStatus = "pending"
	RepurposeStatusProcessing RepurposeJobStatus = "processing"
	RepurposeStatusCompleted  RepurposeJobStatus = "completed"
	RepurposeStatusFailed     RepurposeJobStatus = "failed"
)

// RepurposeJob tracks a single repurposing request.
type RepurposeJob struct {
	ID              uuid.UUID          `json:"id"`
	SourcePostID    uuid.UUID          `json:"source_post_id"`
	UserID          uuid.UUID          `json:"user_id"`
	AIJobID         *uuid.UUID         `json:"ai_job_id,omitempty"`
	Strategy        RepurposeStrategy  `json:"strategy"`
	TargetPlatforms []string           `json:"target_platforms"`
	Options         []byte             `json:"options"`
	Status          RepurposeJobStatus `json:"status"`
	ErrorMessage    *string            `json:"error_message,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

// RepurposeOutput holds the generated content for one target platform.
type RepurposeOutput struct {
	ID                uuid.UUID  `json:"id"`
	RepurposeJobID    uuid.UUID  `json:"repurpose_job_id"`
	TargetPlatform    string     `json:"target_platform"`
	GeneratedText     string     `json:"generated_text"`
	GeneratedHashtags []string   `json:"generated_hashtags"`
	GeneratedTitle    *string    `json:"generated_title,omitempty"`
	MediaSuggestions  []byte     `json:"media_suggestions"`
	Accepted          bool       `json:"accepted"`
	AcceptedPostID    *uuid.UUID `json:"accepted_post_id,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

// CreateRepurposeRequest is the input for starting a repurpose job.
type CreateRepurposeRequest struct {
	SourcePostID    uuid.UUID         `json:"source_post_id"`
	TargetPlatforms []string          `json:"target_platforms"`
	Strategy        string            `json:"strategy"`
	Options         *RepurposeOptions `json:"options,omitempty"`
}

// RepurposeOptions holds optional configuration for the repurpose job.
type RepurposeOptions struct {
	ToneOverride    string     `json:"tone_override,omitempty"`
	MaxLength       int        `json:"max_length,omitempty"`
	IncludeHashtags bool       `json:"include_hashtags"`
	BrandKitID      *uuid.UUID `json:"brand_kit_id,omitempty"`
}

// RepurposeContentRules defines per-strategy source requirements and output format.
var RepurposeContentRules = map[RepurposeStrategy]struct {
	MinSourceLength int
	Description     string
}{
	StrategyLongToShort:    {MinSourceLength: 100, Description: "Condense long-form content into short platform posts"},
	StrategyBlogToThread:   {MinSourceLength: 200, Description: "Convert blog content into a multi-part thread"},
	StrategyVideoToCaption: {MinSourceLength: 20, Description: "Adapt video descriptions into social captions"},
	StrategyPodcastToBlog:  {MinSourceLength: 50, Description: "Transform podcast notes into blog-style content"},
	StrategyCrossPlatform:  {MinSourceLength: 10, Description: "Re-format content for a different platform"},
}
