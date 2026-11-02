package model

import (
	"net"
	"time"

	"github.com/google/uuid"
)

type AuditAction string

const (
	AuditLogin          AuditAction = "login"
	AuditLogout         AuditAction = "logout"
	AuditProfileUpdate  AuditAction = "profile_update"
	AuditBankingUpdate  AuditAction = "banking_update"
	AuditProjectCreate  AuditAction = "project_create"
	AuditProjectUpdate  AuditAction = "project_update"
	AuditProjectClaim   AuditAction = "project_claim"
	AuditVideoUpload    AuditAction = "video_upload"
	AuditReviewComment  AuditAction = "review_comment"
	AuditReviewApprove  AuditAction = "review_approve"
	AuditPayoutApprove  AuditAction = "payout_approve"
	AuditPayoutComplete AuditAction = "payout_complete"
	AuditPostCreate     AuditAction = "post_create"
	AuditPostPublish    AuditAction = "post_publish"
	AuditUserCreate     AuditAction = "user_create"
	AuditUserRoleChange AuditAction = "user_role_change"
	AuditCategoryCreate AuditAction = "category_create"
	AuditChannelCreate  AuditAction = "channel_create"
)

type AuditLog struct {
	ID         uuid.UUID   `json:"id"`
	UserID     *uuid.UUID  `json:"user_id,omitempty"`
	Action     AuditAction `json:"action"`
	EntityType string      `json:"entity_type"`
	EntityID   *uuid.UUID  `json:"entity_id,omitempty"`
	IPAddress  net.IP      `json:"ip_address,omitempty"`
	UserAgent  string      `json:"user_agent,omitempty"`
	RequestID  string      `json:"request_id,omitempty"`
	OldValues  []byte      `json:"old_values,omitempty"`
	NewValues  []byte      `json:"new_values,omitempty"`
	Metadata   []byte      `json:"metadata,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
}
