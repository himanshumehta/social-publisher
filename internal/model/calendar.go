package model

import (
	"time"

	"github.com/google/uuid"
)

// CalendarEntry groups posts by date for the calendar view.
type CalendarEntry struct {
	Date  string             `json:"date"` // "2024-01-15" format
	Posts []CalendarPostItem `json:"posts"`
}

// CalendarPostItem is a lightweight post representation for calendar views.
type CalendarPostItem struct {
	ID              uuid.UUID  `json:"id"`
	Title           string     `json:"title"`
	Status          string     `json:"status"`
	TargetPlatforms []string   `json:"target_platforms"`
	ScheduledAt     *time.Time `json:"scheduled_at,omitempty"`
	PublishedAt     *time.Time `json:"published_at,omitempty"`
	CoverImageURL   string     `json:"cover_image_url,omitempty"`
}

// SchedulePostRequest is the request body for scheduling a post.
type SchedulePostRequest struct {
	ScheduledAt time.Time `json:"scheduled_at"`
	Timezone    string    `json:"timezone"`
}

// BulkScheduleItem represents a single item in a bulk schedule request.
type BulkScheduleItem struct {
	PostID      uuid.UUID `json:"post_id"`
	ScheduledAt time.Time `json:"scheduled_at"`
	Timezone    string    `json:"timezone"`
}

// BulkScheduleRequest is the request body for bulk scheduling.
type BulkScheduleRequest struct {
	Items []BulkScheduleItem `json:"items"`
}

// BulkScheduleResult reports the outcome of scheduling a single post.
type BulkScheduleResult struct {
	PostID  uuid.UUID `json:"post_id"`
	Success bool      `json:"success"`
	Error   string    `json:"error,omitempty"`
}
