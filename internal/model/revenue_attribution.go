package model

import (
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Revenue Source Types
// ---------------------------------------------------------------------------

// RevenueSourceType represents the category of a revenue entry.
type RevenueSourceType string

const (
	SourceTypePlatformAd   RevenueSourceType = "platform_ad"
	SourceTypeSponsorship  RevenueSourceType = "sponsorship"
	SourceTypeAffiliate    RevenueSourceType = "affiliate"
	SourceTypeMerchandise  RevenueSourceType = "merchandise"
	SourceTypeSubscription RevenueSourceType = "subscription"
	SourceTypeDonation     RevenueSourceType = "donation"
	SourceTypeService      RevenueSourceType = "service"
	SourceTypeOther        RevenueSourceType = "other"
)

// ValidSourceTypes is the set of allowed source_type values.
var ValidSourceTypes = map[RevenueSourceType]bool{
	SourceTypePlatformAd:   true,
	SourceTypeSponsorship:  true,
	SourceTypeAffiliate:    true,
	SourceTypeMerchandise:  true,
	SourceTypeSubscription: true,
	SourceTypeDonation:     true,
	SourceTypeService:      true,
	SourceTypeOther:        true,
}

// ---------------------------------------------------------------------------
// Revenue Source
// ---------------------------------------------------------------------------

// RevenueSource represents a single revenue entry from any income stream.
type RevenueSource struct {
	ID          uuid.UUID              `json:"id"`
	UserID      uuid.UUID              `json:"user_id"`
	Name        string                 `json:"name"`
	SourceType  RevenueSourceType      `json:"source_type"`
	Platform    *string                `json:"platform,omitempty"`
	AmountPaise int64                  `json:"amount_paise"`
	Currency    string                 `json:"currency"`
	PeriodStart time.Time              `json:"period_start"`
	PeriodEnd   time.Time              `json:"period_end"`
	Notes       string                 `json:"notes"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Revenue Goal
// ---------------------------------------------------------------------------

// RevenueGoal represents a time-bound revenue target.
type RevenueGoal struct {
	ID                 uuid.UUID  `json:"id"`
	UserID             uuid.UUID  `json:"user_id"`
	Name               string     `json:"name"`
	TargetAmountPaise  int64      `json:"target_amount_paise"`
	CurrentAmountPaise int64      `json:"current_amount_paise"`
	Period             string     `json:"period"`
	PeriodStart        time.Time  `json:"period_start"`
	PeriodEnd          time.Time  `json:"period_end"`
	SourceTypes        []string   `json:"source_types"`
	Status             string     `json:"status"`
	AchievedAt         *time.Time `json:"achieved_at,omitempty"`
	ProgressPercent    float64    `json:"progress_percent"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Affiliate Link
// ---------------------------------------------------------------------------

// AffiliateLink represents a trackable affiliate/referral link.
type AffiliateLink struct {
	ID              uuid.UUID `json:"id"`
	UserID          uuid.UUID `json:"user_id"`
	Name            string    `json:"name"`
	OriginalURL     string    `json:"original_url"`
	TrackingCode    string    `json:"tracking_code"`
	ShortURL        *string   `json:"short_url,omitempty"`
	ClickCount      int       `json:"click_count"`
	ConversionCount int       `json:"conversion_count"`
	RevenuePaise    int64     `json:"revenue_paise"`
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Revenue Breakdown (aggregated view)
// ---------------------------------------------------------------------------

// RevenueBreakdown is the aggregated revenue response for a date range.
type RevenueBreakdown struct {
	TotalPaise  int64            `json:"total_paise"`
	BySource    map[string]int64 `json:"by_source"`
	ByPlatform  map[string]int64 `json:"by_platform"`
	PeriodStart time.Time        `json:"period_start"`
	PeriodEnd   time.Time        `json:"period_end"`
	Growth      float64          `json:"growth"`
}

// ---------------------------------------------------------------------------
// Goal Progress
// ---------------------------------------------------------------------------

// GoalProgress is the detailed progress view for a single revenue goal.
type GoalProgress struct {
	GoalID          uuid.UUID `json:"goal_id"`
	Name            string    `json:"name"`
	TargetPaise     int64     `json:"target_paise"`
	CurrentPaise    int64     `json:"current_paise"`
	ProgressPercent float64   `json:"progress_percent"`
	DaysRemaining   int       `json:"days_remaining"`
	OnTrack         bool      `json:"on_track"`
}

// ---------------------------------------------------------------------------
// Request Types
// ---------------------------------------------------------------------------

// CreateRevenueSourceRequest is the request body for adding a revenue source.
type CreateRevenueSourceRequest struct {
	Name        string            `json:"name"`
	SourceType  RevenueSourceType `json:"source_type"`
	Platform    *string           `json:"platform,omitempty"`
	AmountPaise int64             `json:"amount_paise"`
	Currency    string            `json:"currency"`
	PeriodStart string            `json:"period_start"`
	PeriodEnd   string            `json:"period_end"`
	Notes       string            `json:"notes"`
}

// CreateGoalRequest is the request body for creating a revenue goal.
type CreateGoalRequest struct {
	Name              string   `json:"name"`
	TargetAmountPaise int64    `json:"target_amount_paise"`
	Period            string   `json:"period"`
	SourceTypes       []string `json:"source_types"`
}

// CreateAffiliateLinkRequest is the request body for creating an affiliate link.
type CreateAffiliateLinkRequest struct {
	Name        string `json:"name"`
	OriginalURL string `json:"original_url"`
}
