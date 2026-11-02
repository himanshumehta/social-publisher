package model

import (
	"time"

	"github.com/google/uuid"
)

type ReviewAction string

const (
	ReviewActionComment        ReviewAction = "comment"
	ReviewActionRequestChanges ReviewAction = "request_changes"
	ReviewActionApprove        ReviewAction = "approve"
	ReviewActionReject         ReviewAction = "reject"
	ReviewActionTransition     ReviewAction = "transition"
)

type Review struct {
	ID           uuid.UUID    `json:"id"`
	ProjectID    uuid.UUID    `json:"project_id"`
	VideoID      *uuid.UUID   `json:"video_id,omitempty"`
	ReviewerID   uuid.UUID    `json:"reviewer_id"`
	ParentID     *uuid.UUID   `json:"parent_id,omitempty"`
	Action       ReviewAction `json:"action"`
	Comment      string       `json:"comment"`
	TimestampSec *int         `json:"timestamp_sec,omitempty"`
	FromStatus   string       `json:"from_status,omitempty"`
	ToStatus     string       `json:"to_status,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
	ReviewerName string       `json:"reviewer_name"`
	Replies      []Review     `json:"replies,omitempty"`
}
