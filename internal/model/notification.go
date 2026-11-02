package model

import (
	"time"

	"github.com/google/uuid"
)

type NotificationType string

const (
	NotifPlatformPublish NotificationType = "platform_publish"
	NotifProjectClaimed  NotificationType = "project_claimed"
	NotifReviewComment   NotificationType = "review_comment"
	NotifStatusChange    NotificationType = "status_change"
	NotifPayoutCompleted NotificationType = "payout_completed"
	NotifVideoReady      NotificationType = "video_ready"
	NotifSystem          NotificationType = "system"
)

type Notification struct {
	ID        uuid.UUID        `json:"id"`
	UserID    uuid.UUID        `json:"user_id"`
	Title     string           `json:"title"`
	Body      string           `json:"body,omitempty"`
	Link      string           `json:"link,omitempty"`
	IsRead    bool             `json:"is_read"`
	Type      NotificationType `json:"type"`
	CreatedAt time.Time        `json:"created_at"`
}
