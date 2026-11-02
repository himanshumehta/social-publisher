package model

import (
	"time"

	"github.com/google/uuid"
)

// ── Editing Job ─────────────────────────────────────────────────────────

type EditingJobStatus string

const (
	EditingJobOpen       EditingJobStatus = "open"
	EditingJobInProgress EditingJobStatus = "in_progress"
	EditingJobCompleted  EditingJobStatus = "completed"
	EditingJobCancelled  EditingJobStatus = "cancelled"
)

type EditingJobComplexity string

const (
	ComplexitySimple   EditingJobComplexity = "simple"
	ComplexityStandard EditingJobComplexity = "standard"
	ComplexityComplex  EditingJobComplexity = "complex"
)

type EditingJob struct {
	ID             uuid.UUID            `json:"id"`
	ProjectID      uuid.UUID            `json:"project_id"`
	CreatorID      uuid.UUID            `json:"creator_id"`
	Title          string               `json:"title"`
	Brief          string               `json:"brief"`
	Complexity     EditingJobComplexity `json:"complexity"`
	BudgetMin      int                  `json:"budget_min"`
	BudgetMax      int                  `json:"budget_max"`
	SuggestedPrice int                  `json:"suggested_price"`
	Deadline       *time.Time           `json:"deadline,omitempty"`
	Status         EditingJobStatus     `json:"status"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

// ── Job Application ─────────────────────────────────────────────────────

type ApplicationStatus string

const (
	ApplicationPending     ApplicationStatus = "pending"
	ApplicationShortlisted ApplicationStatus = "shortlisted"
	ApplicationAccepted    ApplicationStatus = "accepted"
	ApplicationRejected    ApplicationStatus = "rejected"
	ApplicationWithdrawn   ApplicationStatus = "withdrawn"
)

type JobApplication struct {
	ID             uuid.UUID         `json:"id"`
	JobID          uuid.UUID         `json:"job_id"`
	EditorID       uuid.UUID         `json:"editor_id"`
	ProposedPrice  int               `json:"proposed_price"`
	CoverNote      string            `json:"cover_note"`
	PortfolioLinks []string          `json:"portfolio_links"`
	Status         ApplicationStatus `json:"status"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// ── Editing Assignment ──────────────────────────────────────────────────

type AssignmentStatus string

const (
	AssignmentAssigned   AssignmentStatus = "assigned"
	AssignmentInProgress AssignmentStatus = "in_progress"
	AssignmentInReview   AssignmentStatus = "in_review"
	AssignmentCompleted  AssignmentStatus = "completed"
	AssignmentDisputed   AssignmentStatus = "disputed"
	AssignmentCancelled  AssignmentStatus = "cancelled"
)

type EditingAssignment struct {
	ID            uuid.UUID        `json:"id"`
	JobID         uuid.UUID        `json:"job_id"`
	ApplicationID uuid.UUID        `json:"application_id"`
	EditorID      uuid.UUID        `json:"editor_id"`
	CreatorID     uuid.UUID        `json:"creator_id"`
	AgreedPrice   int              `json:"agreed_price"`
	PlatformFee   int              `json:"platform_fee"`
	TotalCharged  int              `json:"total_charged"`
	Status        AssignmentStatus `json:"status"`
	DeliveryNotes string           `json:"delivery_notes"`
	RevisionNotes string           `json:"revision_notes"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

// ── Editor Rating ───────────────────────────────────────────────────────

type EditorRating struct {
	ID           uuid.UUID `json:"id"`
	AssignmentID uuid.UUID `json:"assignment_id"`
	RatedBy      uuid.UUID `json:"rated_by"`
	RatedUser    uuid.UUID `json:"rated_user"`
	Rating       int       `json:"rating"`
	ReviewText   string    `json:"review_text"`
	CreatedAt    time.Time `json:"created_at"`
}

// ── Marketplace Transaction ─────────────────────────────────────────────

type MarketplaceTransactionStatus string

const (
	MktTxnPending  MarketplaceTransactionStatus = "pending"
	MktTxnHeld     MarketplaceTransactionStatus = "held"
	MktTxnReleased MarketplaceTransactionStatus = "released"
	MktTxnRefunded MarketplaceTransactionStatus = "refunded"
	MktTxnDisputed MarketplaceTransactionStatus = "disputed"
)

type MarketplaceTransaction struct {
	ID           uuid.UUID                    `json:"id"`
	AssignmentID uuid.UUID                    `json:"assignment_id"`
	CreatorID    uuid.UUID                    `json:"creator_id"`
	EditorID     uuid.UUID                    `json:"editor_id"`
	Amount       int                          `json:"amount"`
	EditorPayout int                          `json:"editor_payout"`
	PlatformFee  int                          `json:"platform_fee"`
	Status       MarketplaceTransactionStatus `json:"status"`
	GatewayRef   string                       `json:"gateway_ref"`
	CreatedAt    time.Time                    `json:"created_at"`
	UpdatedAt    time.Time                    `json:"updated_at"`
}

// ── Editing Price Config ────────────────────────────────────────────────

type EditingPriceConfig struct {
	ID              uuid.UUID `json:"id"`
	DurationBracket string    `json:"duration_bracket"`
	Complexity      string    `json:"complexity"`
	SuggestedPrice  int       `json:"suggested_price"`
}

// ── Stats DTO ───────────────────────────────────────────────────────────

type EditorStats struct {
	Completed   int     `json:"completed"`
	AvgRating   float64 `json:"avg_rating"`
	TotalEarned int     `json:"total_earned"`
}
