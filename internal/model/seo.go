package model

import (
	"time"

	"github.com/google/uuid"
)

type SEOAnalysis struct {
	ID                   uuid.UUID  `json:"id"`
	ProjectID            uuid.UUID  `json:"project_id"`
	PostID               *uuid.UUID `json:"post_id,omitempty"`
	CreatedBy            uuid.UUID  `json:"created_by"`
	Platform             string     `json:"platform"`
	Status               string     `json:"status"`
	OriginalTitle        *string    `json:"original_title,omitempty"`
	OptimizedTitle       *string    `json:"optimized_title,omitempty"`
	OriginalDescription  *string    `json:"original_description,omitempty"`
	OptimizedDescription *string    `json:"optimized_description,omitempty"`
	OriginalTags         []string   `json:"original_tags"`
	OptimizedTags        []string   `json:"optimized_tags"`
	TitleScore           int        `json:"title_score"`
	DescriptionScore     int        `json:"description_score"`
	OverallScore         int        `json:"overall_score"`
	Suggestions          []byte     `json:"suggestions"`
	KeywordData          []byte     `json:"keyword_data"`
	CompetitorData       []byte     `json:"competitor_data"`
	CreditsUsed          int        `json:"credits_used"`
	ErrorMessage         *string    `json:"error_message,omitempty"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type SEOKeyword struct {
	ID             uuid.UUID `json:"id"`
	UserID         uuid.UUID `json:"user_id"`
	Keyword        string    `json:"keyword"`
	SearchVolume   int       `json:"search_volume"`
	Competition    string    `json:"competition"`
	TrendDirection string    `json:"trend_direction"`
	RelevanceScore float32   `json:"relevance_score"`
	Platform       string    `json:"platform"`
	Category       *string   `json:"category,omitempty"`
	LastUpdatedAt  time.Time `json:"last_updated_at"`
	CreatedAt      time.Time `json:"created_at"`
}

type ContentSuggestion struct {
	ID             uuid.UUID `json:"id"`
	UserID         uuid.UUID `json:"user_id"`
	SuggestionType string    `json:"suggestion_type"`
	Title          string    `json:"title"`
	Description    *string   `json:"description,omitempty"`
	Keywords       []string  `json:"keywords"`
	EstimatedViews *int      `json:"estimated_views,omitempty"`
	Confidence     float32   `json:"confidence"`
	TrendingScore  int       `json:"trending_score"`
	Source         *string   `json:"source,omitempty"`
	Metadata       []byte    `json:"metadata,omitempty"`
	IsDismissed    bool      `json:"is_dismissed"`
	CreatedAt      time.Time `json:"created_at"`
}

type CompetitorChannel struct {
	ID                uuid.UUID  `json:"id"`
	UserID            uuid.UUID  `json:"user_id"`
	Platform          string     `json:"platform"`
	ChannelName       string     `json:"channel_name"`
	ChannelURL        *string    `json:"channel_url,omitempty"`
	PlatformChannelID *string    `json:"platform_channel_id,omitempty"`
	SubscriberCount   *int       `json:"subscriber_count,omitempty"`
	VideoCount        *int       `json:"video_count,omitempty"`
	AvgViews          *int       `json:"avg_views,omitempty"`
	TopKeywords       []string   `json:"top_keywords"`
	LastAnalyzedAt    *time.Time `json:"last_analyzed_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type AnalyzeSEORequest struct {
	ProjectID   uuid.UUID  `json:"project_id" validate:"required"`
	PostID      *uuid.UUID `json:"post_id,omitempty"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Tags        []string   `json:"tags"`
	Platform    string     `json:"platform"`
}

type TrackKeywordRequest struct {
	Keyword  string `json:"keyword" validate:"required"`
	Platform string `json:"platform"`
	Category string `json:"category"`
}

type AddCompetitorRequest struct {
	ChannelName       string `json:"channel_name" validate:"required"`
	ChannelURL        string `json:"channel_url"`
	PlatformChannelID string `json:"platform_channel_id"`
	Platform          string `json:"platform"`
}

type CreateContentSuggestionRequest struct {
	SuggestionType string   `json:"suggestion_type" validate:"required"`
	Title          string   `json:"title" validate:"required"`
	Description    string   `json:"description"`
	Keywords       []string `json:"keywords"`
	EstimatedViews int      `json:"estimated_views"`
	TrendingScore  int      `json:"trending_score"`
}

// SEOScoreInput contains data for scoring title/description SEO quality.
type SEOScoreInput struct {
	Title          string
	Description    string
	Tags           []string
	Platform       string
	TargetKeywords []string
}

// SEOScoreResult contains the scoring output.
type SEOScoreResult struct {
	TitleScore       int      `json:"title_score"`
	DescriptionScore int      `json:"description_score"`
	OverallScore     int      `json:"overall_score"`
	TitleSuggestions []string `json:"title_suggestions"`
	DescSuggestions  []string `json:"desc_suggestions"`
	TagSuggestions   []string `json:"tag_suggestions"`
}
