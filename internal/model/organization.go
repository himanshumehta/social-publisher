package model

import (
	"time"

	"github.com/google/uuid"
)

// ── Plan types ──────────────────────────────────────────────────────────

type OrgPlan string

const (
	OrgPlanStarter      OrgPlan = "starter"
	OrgPlanProfessional OrgPlan = "professional"
	OrgPlanEnterprise   OrgPlan = "enterprise"
)

// ValidOrgPlans is the set of valid organization plan values.
var ValidOrgPlans = map[OrgPlan]bool{
	OrgPlanStarter:      true,
	OrgPlanProfessional: true,
	OrgPlanEnterprise:   true,
}

// ── Org role types ──────────────────────────────────────────────────────

type OrgRole string

const (
	OrgRoleOwner   OrgRole = "owner"
	OrgRoleAdmin   OrgRole = "admin"
	OrgRoleManager OrgRole = "manager"
	OrgRoleMember  OrgRole = "member"
	OrgRoleViewer  OrgRole = "viewer"
)

// ValidOrgRoles is the set of valid organization role values.
var ValidOrgRoles = map[OrgRole]bool{
	OrgRoleOwner:   true,
	OrgRoleAdmin:   true,
	OrgRoleManager: true,
	OrgRoleMember:  true,
	OrgRoleViewer:  true,
}

// OrgRoleRank returns a numeric rank for role comparison.
// Higher rank = more permissions.
func OrgRoleRank(role OrgRole) int {
	switch role {
	case OrgRoleOwner:
		return 50
	case OrgRoleAdmin:
		return 40
	case OrgRoleManager:
		return 30
	case OrgRoleMember:
		return 20
	case OrgRoleViewer:
		return 10
	default:
		return 0
	}
}

// ── Org member status ───────────────────────────────────────────────────

type OrgMemberStatus string

const (
	OrgMemberPending   OrgMemberStatus = "pending"
	OrgMemberActive    OrgMemberStatus = "active"
	OrgMemberSuspended OrgMemberStatus = "suspended"
	OrgMemberRemoved   OrgMemberStatus = "removed"
)

// ── Subscription status ─────────────────────────────────────────────────

type OrgSubStatus string

const (
	OrgSubActive    OrgSubStatus = "active"
	OrgSubPastDue   OrgSubStatus = "past_due"
	OrgSubCancelled OrgSubStatus = "cancelled"
	OrgSubTrialing  OrgSubStatus = "trialing"
)

// ── Invoice status ──────────────────────────────────────────────────────

type OrgInvoiceStatus string

const (
	OrgInvoiceDraft   OrgInvoiceStatus = "draft"
	OrgInvoiceSent    OrgInvoiceStatus = "sent"
	OrgInvoicePaid    OrgInvoiceStatus = "paid"
	OrgInvoiceOverdue OrgInvoiceStatus = "overdue"
	OrgInvoiceVoid    OrgInvoiceStatus = "void"
)

// ── Core structs ────────────────────────────────────────────────────────

type Organization struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	LogoURL     string    `json:"logo_url,omitempty"`
	Domain      string    `json:"domain,omitempty"`
	OwnerID     uuid.UUID `json:"owner_id"`
	Plan        OrgPlan   `json:"plan"`
	MaxMembers  int       `json:"max_members"`
	MaxProjects int       `json:"max_projects"`
	Settings    []byte    `json:"settings"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type OrgMember struct {
	ID        uuid.UUID       `json:"id"`
	OrgID     uuid.UUID       `json:"org_id"`
	UserID    uuid.UUID       `json:"user_id"`
	Role      OrgRole         `json:"role"`
	InvitedBy *uuid.UUID      `json:"invited_by,omitempty"`
	Status    OrgMemberStatus `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`

	// Joined from users table for display
	UserName  string `json:"user_name,omitempty"`
	UserEmail string `json:"user_email,omitempty"`
}

type OrgSubscription struct {
	ID                    uuid.UUID    `json:"id"`
	OrgID                 uuid.UUID    `json:"org_id"`
	Plan                  OrgPlan      `json:"plan"`
	AmountPaise           int64        `json:"amount_paise"`
	BillingCycle          string       `json:"billing_cycle"`
	Status                OrgSubStatus `json:"status"`
	CurrentPeriodStart    time.Time    `json:"current_period_start"`
	CurrentPeriodEnd      time.Time    `json:"current_period_end"`
	GatewaySubscriptionID *string      `json:"gateway_subscription_id,omitempty"`
	CreatedAt             time.Time    `json:"created_at"`
	UpdatedAt             time.Time    `json:"updated_at"`
}

type OrgInvoice struct {
	ID             uuid.UUID        `json:"id"`
	OrgID          uuid.UUID        `json:"org_id"`
	SubscriptionID *uuid.UUID       `json:"subscription_id,omitempty"`
	AmountPaise    int64            `json:"amount_paise"`
	TaxPaise       int64            `json:"tax_paise"`
	TotalPaise     int64            `json:"total_paise"`
	Status         OrgInvoiceStatus `json:"status"`
	InvoiceNumber  string           `json:"invoice_number"`
	PeriodStart    time.Time        `json:"period_start"`
	PeriodEnd      time.Time        `json:"period_end"`
	PaidAt         *time.Time       `json:"paid_at,omitempty"`
	DueDate        time.Time        `json:"due_date"`
	CreatedAt      time.Time        `json:"created_at"`
}

// ── Request / Response structs ──────────────────────────────────────────

type CreateOrgRequest struct {
	Name    string  `json:"name"`
	Slug    string  `json:"slug,omitempty"` // auto-generated if empty
	LogoURL string  `json:"logo_url,omitempty"`
	Domain  string  `json:"domain,omitempty"`
	Plan    OrgPlan `json:"plan,omitempty"` // defaults to starter
}

type InviteOrgMemberRequest struct {
	Email string  `json:"email"`
	Role  OrgRole `json:"role"`
}

type UpdateOrgMemberRoleRequest struct {
	Role OrgRole `json:"role"`
}

// ── Dashboard ───────────────────────────────────────────────────────────

type OrgDashboard struct {
	MemberCount        int              `json:"member_count"`
	ProjectCount       int              `json:"project_count"`
	PostCount          int              `json:"post_count"`
	ActiveSubscription *OrgSubscription `json:"active_subscription,omitempty"`
	MonthlySpend       int64            `json:"monthly_spend"`
	RecentActivity     []OrgActivity    `json:"recent_activity"`
}

type OrgActivity struct {
	Type        string    `json:"type"`
	ActorName   string    `json:"actor_name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// ── Plan limits ─────────────────────────────────────────────────────────

type PlanLimits struct {
	MaxMembers  int
	MaxProjects int
}

// PlanLimitsMap defines the limits for each plan tier.
// Enterprise uses -1 to represent unlimited.
var PlanLimitsMap = map[OrgPlan]PlanLimits{
	OrgPlanStarter:      {MaxMembers: 5, MaxProjects: 10},
	OrgPlanProfessional: {MaxMembers: 25, MaxProjects: 50},
	OrgPlanEnterprise:   {MaxMembers: -1, MaxProjects: -1}, // unlimited
}
