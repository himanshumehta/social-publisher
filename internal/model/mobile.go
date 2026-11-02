package model

import (
	"time"

	"github.com/google/uuid"
)

// PushToken represents a device registered for push notifications.
type PushToken struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	DeviceID   string    `json:"device_id"`
	Platform   string    `json:"platform"`
	Token      string    `json:"token"`
	IsActive   bool      `json:"is_active"`
	LastUsedAt time.Time `json:"last_used_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// PushNotification represents an entry in the push delivery queue.
type PushNotification struct {
	ID           uuid.UUID  `json:"id"`
	UserID       uuid.UUID  `json:"user_id"`
	Title        string     `json:"title"`
	Body         string     `json:"body"`
	Data         []byte     `json:"data"`
	Priority     string     `json:"priority"`
	Status       string     `json:"status"`
	ErrorMessage string     `json:"error_message,omitempty"`
	SentAt       *time.Time `json:"sent_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// SyncEntry represents a single change in the offline sync queue.
type SyncEntry struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	EntityType  string    `json:"entity_type"`
	EntityID    uuid.UUID `json:"entity_id"`
	Action      string    `json:"action"`
	Payload     []byte    `json:"payload"`
	SequenceNum int64     `json:"sequence_num"`
	CreatedAt   time.Time `json:"created_at"`
}

// RegisterTokenRequest is the input for registering a push token.
type RegisterTokenRequest struct {
	DeviceID string `json:"device_id"`
	Platform string `json:"platform"`
	Token    string `json:"token"`
}

// QuickPostRequest is a simplified post creation for mobile clients.
type QuickPostRequest struct {
	Title           string   `json:"title"`
	Content         string   `json:"content"`
	TargetPlatforms []string `json:"target_platforms"`
	AutoPublish     bool     `json:"auto_publish"`
}

// SyncRequest carries the client cursor for delta sync.
type SyncRequest struct {
	LastSequence int64 `json:"last_sequence"`
}

// SyncResponse is the cursor-based sync response.
type SyncResponse struct {
	Entries      []SyncEntry `json:"entries"`
	LastSequence int64       `json:"last_sequence"`
	HasMore      bool        `json:"has_more"`
}

// NotificationFeedItem is a mobile-friendly notification view.
type NotificationFeedItem struct {
	ID        uuid.UUID  `json:"id"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Type      string     `json:"type"`
	EntityID  *uuid.UUID `json:"entity_id,omitempty"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// MobileDashboard is the aggregated dashboard for mobile home screens.
type MobileDashboard struct {
	PostCount           int              `json:"post_count"`
	DraftCount          int              `json:"draft_count"`
	ScheduledCount      int              `json:"scheduled_count"`
	PublishedCount      int              `json:"published_count"`
	UnreadNotifications int              `json:"unread_notifications"`
	RecentPosts         []MobilePostItem `json:"recent_posts"`
}

// MobilePostItem is a lightweight post view for mobile lists.
type MobilePostItem struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Platform  string    `json:"platform"`
	CreatedAt time.Time `json:"created_at"`
}
