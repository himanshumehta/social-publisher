package model

import (
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// A/B Test Status
// ---------------------------------------------------------------------------

type ABTestStatus string

const (
	ABTestStatusDraft      ABTestStatus = "draft"
	ABTestStatusRunning    ABTestStatus = "running"
	ABTestStatusEvaluating ABTestStatus = "evaluating"
	ABTestStatusCompleted  ABTestStatus = "completed"
	ABTestStatusCancelled  ABTestStatus = "cancelled"
)

// ---------------------------------------------------------------------------
// A/B Test Metric
// ---------------------------------------------------------------------------

type ABTestMetric string

const (
	ABTestMetricEngagement  ABTestMetric = "engagement"
	ABTestMetricClicks      ABTestMetric = "clicks"
	ABTestMetricImpressions ABTestMetric = "impressions"
	ABTestMetricReach       ABTestMetric = "reach"
)

// ---------------------------------------------------------------------------
// Core structs
// ---------------------------------------------------------------------------

// ABTest represents a single A/B test comparing post variants.
type ABTest struct {
	ID              uuid.UUID       `json:"id"`
	UserID          uuid.UUID       `json:"user_id"`
	OriginalPostID  uuid.UUID       `json:"original_post_id"`
	Name            string          `json:"name"`
	Status          ABTestStatus    `json:"status"`
	Metric          ABTestMetric    `json:"metric"`
	DurationHours   int             `json:"duration_hours"`
	StartedAt       *time.Time      `json:"started_at,omitempty"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty"`
	WinnerVariantID *uuid.UUID      `json:"winner_variant_id,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	Variants        []ABTestVariant `json:"variants"`
}

// ABTestVariant represents one arm of an A/B test.
type ABTestVariant struct {
	ID             uuid.UUID `json:"id"`
	ABTestID       uuid.UUID `json:"ab_test_id"`
	VariantLabel   string    `json:"variant_label"`
	PostID         uuid.UUID `json:"post_id"`
	IsControl      bool      `json:"is_control"`
	Views          int       `json:"views"`
	Likes          int       `json:"likes"`
	Comments       int       `json:"comments"`
	Shares         int       `json:"shares"`
	Clicks         int       `json:"clicks"`
	Impressions    int       `json:"impressions"`
	EngagementRate float64   `json:"engagement_rate"`
	IsWinner       bool      `json:"is_winner"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Request / Response DTOs
// ---------------------------------------------------------------------------

// CreateABTestRequest is the input for creating a new A/B test.
type CreateABTestRequest struct {
	OriginalPostID uuid.UUID            `json:"original_post_id"`
	Name           string               `json:"name"`
	Variants       []ABTestVariantInput `json:"variants"`
	Metric         ABTestMetric         `json:"metric"`
	DurationHours  int                  `json:"duration_hours"`
}

// ABTestVariantInput describes a single variant when creating a test.
type ABTestVariantInput struct {
	VariantLabel string    `json:"variant_label"`
	PostID       uuid.UUID `json:"post_id"`
}

// ABTestResult is returned after evaluating a test.
type ABTestResult struct {
	TestID          uuid.UUID `json:"test_id"`
	WinnerVariantID uuid.UUID `json:"winner_variant_id"`
	WinnerLabel     string    `json:"winner_label"`
	Improvement     float64   `json:"improvement"`
	Confidence      float64   `json:"confidence"`
}

// SelectWinnerRequest is the body for manually selecting a winner.
type SelectWinnerRequest struct {
	VariantID uuid.UUID `json:"variant_id"`
}
