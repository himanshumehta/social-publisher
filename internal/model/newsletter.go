package model

import (
	"time"

	"github.com/google/uuid"
)

type Newsletter struct {
	ID              uuid.UUID `json:"id"`
	UserID          uuid.UUID `json:"user_id"`
	Name            string    `json:"name"`
	Description     *string   `json:"description,omitempty"`
	FromName        *string   `json:"from_name,omitempty"`
	FromEmail       *string   `json:"from_email,omitempty"`
	ReplyTo         *string   `json:"reply_to,omitempty"`
	LogoURL         *string   `json:"logo_url,omitempty"`
	ColorPrimary    string    `json:"color_primary"`
	ColorSecondary  string    `json:"color_secondary"`
	FooterText      *string   `json:"footer_text,omitempty"`
	IsActive        bool      `json:"is_active"`
	SubscriberCount int       `json:"subscriber_count"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type NewsletterSubscriber struct {
	ID             uuid.UUID  `json:"id"`
	NewsletterID   uuid.UUID  `json:"newsletter_id"`
	Email          string     `json:"email"`
	Name           *string    `json:"name,omitempty"`
	Status         string     `json:"status"`
	Source         *string    `json:"source,omitempty"`
	Metadata       []byte     `json:"metadata,omitempty"`
	SubscribedAt   time.Time  `json:"subscribed_at"`
	UnsubscribedAt *time.Time `json:"unsubscribed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type NewsletterIssue struct {
	ID               uuid.UUID  `json:"id"`
	NewsletterID     uuid.UUID  `json:"newsletter_id"`
	Subject          string     `json:"subject"`
	PreviewText      *string    `json:"preview_text,omitempty"`
	HTMLContent      string     `json:"html_content"`
	PlainTextContent *string    `json:"plain_text_content,omitempty"`
	Status           string     `json:"status"`
	SourcePostID     *uuid.UUID `json:"source_post_id,omitempty"`
	SourceProjectID  *uuid.UUID `json:"source_project_id,omitempty"`
	SentCount        int        `json:"sent_count"`
	OpenCount        int        `json:"open_count"`
	ClickCount       int        `json:"click_count"`
	BounceCount      int        `json:"bounce_count"`
	ScheduledAt      *time.Time `json:"scheduled_at,omitempty"`
	SentAt           *time.Time `json:"sent_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type NewsletterEvent struct {
	ID           uuid.UUID `json:"id"`
	IssueID      uuid.UUID `json:"issue_id"`
	SubscriberID uuid.UUID `json:"subscriber_id"`
	EventType    string    `json:"event_type"`
	Metadata     []byte    `json:"metadata,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type CreateNewsletterRequest struct {
	Name           string `json:"name" validate:"required"`
	Description    string `json:"description"`
	FromName       string `json:"from_name"`
	FromEmail      string `json:"from_email"`
	ReplyTo        string `json:"reply_to"`
	ColorPrimary   string `json:"color_primary"`
	ColorSecondary string `json:"color_secondary"`
	FooterText     string `json:"footer_text"`
}

type UpdateNewsletterRequest struct {
	Name           *string `json:"name,omitempty"`
	Description    *string `json:"description,omitempty"`
	FromName       *string `json:"from_name,omitempty"`
	FromEmail      *string `json:"from_email,omitempty"`
	ReplyTo        *string `json:"reply_to,omitempty"`
	ColorPrimary   *string `json:"color_primary,omitempty"`
	ColorSecondary *string `json:"color_secondary,omitempty"`
	FooterText     *string `json:"footer_text,omitempty"`
	IsActive       *bool   `json:"is_active,omitempty"`
}

type AddSubscriberRequest struct {
	Email  string `json:"email" validate:"required"`
	Name   string `json:"name"`
	Source string `json:"source"`
}

type BulkAddSubscribersRequest struct {
	Subscribers []AddSubscriberRequest `json:"subscribers" validate:"required,min=1"`
}

type CreateIssueRequest struct {
	Subject          string     `json:"subject" validate:"required"`
	PreviewText      string     `json:"preview_text"`
	HTMLContent      string     `json:"html_content" validate:"required"`
	PlainTextContent string     `json:"plain_text_content"`
	SourcePostID     *uuid.UUID `json:"source_post_id"`
	SourceProjectID  *uuid.UUID `json:"source_project_id"`
}

type NewsletterStats struct {
	TotalSubscribers  int     `json:"total_subscribers"`
	ActiveSubscribers int     `json:"active_subscribers"`
	TotalIssuesSent   int     `json:"total_issues_sent"`
	AvgOpenRate       float64 `json:"avg_open_rate"`
	AvgClickRate      float64 `json:"avg_click_rate"`
}
