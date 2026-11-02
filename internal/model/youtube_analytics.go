package model

import (
	"time"

	"github.com/google/uuid"
)

type YouTubeAnalytics struct {
	ID                    uuid.UUID  `json:"id"`
	PublishRecordID       uuid.UUID  `json:"publish_record_id"`
	YouTubeVideoID        string     `json:"youtube_video_id"`
	PeriodStart           time.Time  `json:"period_start"`
	PeriodEnd             time.Time  `json:"period_end"`
	Views                 int64      `json:"views"`
	WatchTimeMinutes      int64      `json:"watch_time_minutes"`
	Likes                 int        `json:"likes"`
	CommentsCount         int        `json:"comments_count"`
	Shares                int        `json:"shares"`
	SubscribersGained     int        `json:"subscribers_gained"`
	EstimatedRevenuePaise int64      `json:"estimated_revenue_paise"`
	AdRevenuePaise        int64      `json:"ad_revenue_paise"`
	RawResponse           []byte     `json:"raw_response,omitempty"`
	FetchedAt             *time.Time `json:"fetched_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
}
