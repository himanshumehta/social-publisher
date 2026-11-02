package model

import (
	"time"

	"github.com/google/uuid"
)

// AnalyticsSnapshot is the row-level representation of analytics_snapshots.
type AnalyticsSnapshot struct {
	ID             uuid.UUID              `json:"id"`
	UserID         uuid.UUID              `json:"user_id"`
	ChannelID      *uuid.UUID             `json:"channel_id,omitempty"`
	Platform       string                 `json:"platform"`
	SnapshotDate   time.Time              `json:"snapshot_date"`
	Followers      int                    `json:"followers"`
	Views          int                    `json:"views"`
	Likes          int                    `json:"likes"`
	Comments       int                    `json:"comments"`
	Shares         int                    `json:"shares"`
	EngagementRate float64                `json:"engagement_rate"`
	Impressions    int                    `json:"impressions"`
	Reach          int                    `json:"reach"`
	Clicks         int                    `json:"clicks"`
	RevenuePaise   int64                  `json:"revenue_paise"`
	Metadata       map[string]interface{} `json:"metadata"`
	CreatedAt      time.Time              `json:"created_at"`
}

// PeriodGrowth holds percentage growth between two periods.
type PeriodGrowth struct {
	Followers   float64 `json:"followers"`
	Views       float64 `json:"views"`
	Engagements float64 `json:"engagements"`
}

// PlatformStat is a per-platform aggregate within a date range.
type PlatformStat struct {
	Platform       string  `json:"platform"`
	Followers      int     `json:"followers"`
	Views          int     `json:"views"`
	Likes          int     `json:"likes"`
	Comments       int     `json:"comments"`
	Shares         int     `json:"shares"`
	EngagementRate float64 `json:"engagement_rate"`
	Revenue        int64   `json:"revenue"`
}

// AnalyticsOverview is the top-level dashboard payload.
type AnalyticsOverview struct {
	TotalFollowers    int            `json:"total_followers"`
	TotalViews        int            `json:"total_views"`
	TotalEngagements  int            `json:"total_engagements"`
	EngagementRate    float64        `json:"engagement_rate"`
	PeriodGrowth      PeriodGrowth   `json:"period_growth"`
	PlatformBreakdown []PlatformStat `json:"platform_breakdown"`
}

// PostAnalytics holds per-platform metrics for a single post.
type PostAnalytics struct {
	PostID         uuid.UUID `json:"post_id"`
	Platform       string    `json:"platform"`
	Views          int       `json:"views"`
	Likes          int       `json:"likes"`
	Comments       int       `json:"comments"`
	Shares         int       `json:"shares"`
	Clicks         int       `json:"clicks"`
	EngagementRate float64   `json:"engagement_rate"`
}

// AggregatedMetrics is used inside period comparisons.
type AggregatedMetrics struct {
	Followers   int     `json:"followers"`
	Views       int     `json:"views"`
	Likes       int     `json:"likes"`
	Comments    int     `json:"comments"`
	Shares      int     `json:"shares"`
	Engagements int     `json:"engagements"`
	Revenue     int64   `json:"revenue"`
	EngRate     float64 `json:"engagement_rate"`
}

// AnalyticsComparison compares two arbitrary date ranges.
type AnalyticsComparison struct {
	CurrentPeriod  AggregatedMetrics `json:"current_period"`
	PreviousPeriod AggregatedMetrics `json:"previous_period"`
}

// DateRange is a reusable half-open [Start, End) range.
type DateRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}
