package model

import (
	"time"

	"github.com/google/uuid"
)

// ── Team Role ───────────────────────────────────────────────────────────

type TeamRole string

const (
	TeamRoleViewer      TeamRole = "viewer"
	TeamRoleContributor TeamRole = "contributor"
	TeamRoleManager     TeamRole = "manager"
	TeamRoleAdmin       TeamRole = "admin"
)

var ValidTeamRoles = map[TeamRole]bool{
	TeamRoleViewer:      true,
	TeamRoleContributor: true,
	TeamRoleManager:     true,
	TeamRoleAdmin:       true,
}

// ── Team Member Status ──────────────────────────────────────────────────

type TeamMemberStatus string

const (
	TeamMemberPending TeamMemberStatus = "pending"
	TeamMemberActive  TeamMemberStatus = "active"
	TeamMemberRemoved TeamMemberStatus = "removed"
)

// ── Team Member ─────────────────────────────────────────────────────────

type TeamMember struct {
	ID           uuid.UUID        `json:"id"`
	OwnerID      uuid.UUID        `json:"owner_id"`
	MemberID     uuid.UUID        `json:"member_id"`
	Role         TeamRole         `json:"role"`
	InvitedBy    uuid.UUID        `json:"invited_by"`
	InvitedEmail string           `json:"invited_email,omitempty"`
	Status       TeamMemberStatus `json:"status"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`

	// Display fields (populated via JOIN)
	MemberName  string `json:"member_name"`
	MemberEmail string `json:"member_email"`
}

// ── Team Request DTOs ───────────────────────────────────────────────────

type InviteMemberRequest struct {
	Email string   `json:"email"`
	Role  TeamRole `json:"role"`
}

type UpdateMemberRoleRequest struct {
	Role TeamRole `json:"role"`
}

// ── Approval Status ─────────────────────────────────────────────────────

type ApprovalStatus string

const (
	ApprovalPending          ApprovalStatus = "pending"
	ApprovalInReview         ApprovalStatus = "in_review"
	ApprovalApproved         ApprovalStatus = "approved"
	ApprovalRejected         ApprovalStatus = "rejected"
	ApprovalChangesRequested ApprovalStatus = "changes_requested"
)

// ── Approval Action Type ────────────────────────────────────────────────

type ApprovalAction string

const (
	ActionApprove        ApprovalAction = "approve"
	ActionReject         ApprovalAction = "reject"
	ActionRequestChanges ApprovalAction = "request_changes"
)

var ValidApprovalActions = map[ApprovalAction]bool{
	ActionApprove:        true,
	ActionReject:         true,
	ActionRequestChanges: true,
}

// ── Workflow Step ────────────────────────────────────────────────────────

type WorkflowStep struct {
	StepNumber   int        `json:"step_number"`
	ApproverRole string     `json:"approver_role"`
	ApproverID   *uuid.UUID `json:"approver_id,omitempty"`
	Required     bool       `json:"required"`
}

// ── Approval Workflow ───────────────────────────────────────────────────

type ApprovalWorkflow struct {
	ID        uuid.UUID      `json:"id"`
	OwnerID   uuid.UUID      `json:"owner_id"`
	Name      string         `json:"name"`
	Steps     []WorkflowStep `json:"steps"`
	IsActive  bool           `json:"is_active"`
	AutoApply bool           `json:"auto_apply"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// ── Approval Request ────────────────────────────────────────────────────

type ApprovalRequest struct {
	ID          uuid.UUID              `json:"id"`
	PostID      uuid.UUID              `json:"post_id"`
	WorkflowID  uuid.UUID              `json:"workflow_id"`
	SubmittedBy uuid.UUID              `json:"submitted_by"`
	CurrentStep int                    `json:"current_step"`
	Status      ApprovalStatus         `json:"status"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Actions     []ApprovalActionRecord `json:"actions,omitempty"`
}

// ── Approval Action Record ──────────────────────────────────────────────

type ApprovalActionRecord struct {
	ID         uuid.UUID      `json:"id"`
	RequestID  uuid.UUID      `json:"request_id"`
	StepNumber int            `json:"step_number"`
	ActorID    uuid.UUID      `json:"actor_id"`
	Action     ApprovalAction `json:"action"`
	Comment    string         `json:"comment"`
	CreatedAt  time.Time      `json:"created_at"`
}

// ── Approval Request DTOs ───────────────────────────────────────────────

type CreateWorkflowRequest struct {
	Name      string         `json:"name"`
	Steps     []WorkflowStep `json:"steps"`
	AutoApply bool           `json:"auto_apply"`
}

type SubmitApprovalRequest struct {
	WorkflowID *uuid.UUID `json:"workflow_id,omitempty"`
}

type ApprovalActionRequest struct {
	Action  ApprovalAction `json:"action"`
	Comment string         `json:"comment"`
}
