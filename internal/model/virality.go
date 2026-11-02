package model

import (
	"time"

	"github.com/google/uuid"
)

// ViralityScore is the persisted scoring record for a post.
type ViralityScore struct {
	ID          uuid.UUID      `json:"id"`
	PostID      uuid.UUID      `json:"post_id"`
	UserID      uuid.UUID      `json:"user_id"`
	Score       int            `json:"score"`
	Factors     ScoringFactors `json:"factors"`
	Suggestions []string       `json:"suggestions"`
	ScoredAt    time.Time      `json:"scored_at"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// ScoringFactors breaks the total score into its component parts.
// Each field represents the points awarded for that factor.
type ScoringFactors struct {
	HashtagScore           int `json:"hashtag_score"`            // 0-20
	TimingScore            int `json:"timing_score"`             // 0-20
	ContentLengthScore     int `json:"content_length_score"`     // 0-15
	EngagementHistoryScore int `json:"engagement_history_score"` // 0-25
	MediaRichnessScore     int `json:"media_richness_score"`     // 0-10
	PlatformMatchScore     int `json:"platform_match_score"`     // 0-10
}

// TrendingHashtag tracks a hashtag's popularity on a specific platform.
type TrendingHashtag struct {
	ID          uuid.UUID `json:"id"`
	Hashtag     string    `json:"hashtag"`
	Platform    string    `json:"platform"`
	TrendScore  float64   `json:"trend_score"`
	SampleCount int       `json:"sample_count"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
}

// ScoreResult is the output of the scoring engine (not yet persisted).
type ScoreResult struct {
	Score       int            `json:"score"`
	Factors     ScoringFactors `json:"factors"`
	Suggestions []string       `json:"suggestions"`
}

// PostScoringInput contains all the data the scoring engine needs.
// It is assembled by the service layer from the post and related data.
type PostScoringInput struct {
	Content             string   `json:"content"`
	Hashtags            []string `json:"hashtags"`
	TargetPlatforms     []string `json:"target_platforms"`
	HasImage            bool     `json:"has_image"`
	HasVideo            bool     `json:"has_video"`
	HasLink             bool     `json:"has_link"`
	PostingHour         int      `json:"posting_hour"`          // 0-23
	PostingWeekday      int      `json:"posting_weekday"`       // 0=Sunday, 6=Saturday
	AuthorAvgEngagement float64  `json:"author_avg_engagement"` // 0.0-100.0 percent
	ContentLength       int      `json:"content_length"`
}
