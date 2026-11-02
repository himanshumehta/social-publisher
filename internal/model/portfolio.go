package model

import (
	"time"

	"github.com/google/uuid"
)

// ── Editor Portfolio ────────────────────────────────────────────────────

type AvailabilityStatus string

const (
	AvailabilityAvailable   AvailabilityStatus = "available"
	AvailabilityBusy        AvailabilityStatus = "busy"
	AvailabilityUnavailable AvailabilityStatus = "unavailable"
)

type EditorPortfolio struct {
	ID              uuid.UUID          `json:"id"`
	EditorID        uuid.UUID          `json:"editor_id"`
	Bio             string             `json:"bio"`
	Specializations []string           `json:"specializations"`
	YearsExperience int                `json:"years_experience"`
	HourlyRate      int                `json:"hourly_rate"`
	Availability    AvailabilityStatus `json:"availability"`
	PortfolioURL    string             `json:"portfolio_url"`
	DemoReelURL     string             `json:"demo_reel_url"`
	IsFeatured      bool               `json:"is_featured"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`

	// Loaded via joins
	Skills []EditorSkill   `json:"skills"`
	Items  []PortfolioItem `json:"items"`
}

// ── Portfolio Item ──────────────────────────────────────────────────────

type MediaType string

const (
	MediaTypeVideo MediaType = "video"
	MediaTypeImage MediaType = "image"
	MediaTypeLink  MediaType = "link"
)

type PortfolioItem struct {
	ID           uuid.UUID `json:"id"`
	PortfolioID  uuid.UUID `json:"portfolio_id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	ThumbnailURL string    `json:"thumbnail_url"`
	MediaURL     string    `json:"media_url"`
	MediaType    MediaType `json:"media_type"`
	Platform     string    `json:"platform"`
	ViewCount    int       `json:"view_count"`
	SortOrder    int       `json:"sort_order"`
	CreatedAt    time.Time `json:"created_at"`
}

// ── Editor Skill ────────────────────────────────────────────────────────

type ProficiencyLevel string

const (
	ProficiencyBeginner     ProficiencyLevel = "beginner"
	ProficiencyIntermediate ProficiencyLevel = "intermediate"
	ProficiencyAdvanced     ProficiencyLevel = "advanced"
	ProficiencyExpert       ProficiencyLevel = "expert"
)

type EditorSkill struct {
	ID          uuid.UUID        `json:"id"`
	EditorID    uuid.UUID        `json:"editor_id"`
	SkillName   string           `json:"skill_name"`
	Proficiency ProficiencyLevel `json:"proficiency"`
	Verified    bool             `json:"verified"`
}

// ── Marketplace Template ────────────────────────────────────────────────

type MarketplaceTemplate struct {
	ID            uuid.UUID `json:"id"`
	CreatorID     uuid.UUID `json:"creator_id"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	Category      string    `json:"category"`
	BriefTemplate string    `json:"brief_template"`
	Price         int       `json:"price"`
	PurchaseCount int       `json:"purchase_count"`
	RatingAvg     float64   `json:"rating_avg"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ── Request / Response DTOs ─────────────────────────────────────────────

type CreatePortfolioRequest struct {
	Bio             string   `json:"bio"`
	Specializations []string `json:"specializations"`
	YearsExperience int      `json:"years_experience"`
	HourlyRate      int      `json:"hourly_rate"`
	Availability    string   `json:"availability"`
	PortfolioURL    string   `json:"portfolio_url"`
	DemoReelURL     string   `json:"demo_reel_url"`
}

type UpdatePortfolioRequest struct {
	Bio             *string   `json:"bio,omitempty"`
	Specializations *[]string `json:"specializations,omitempty"`
	YearsExperience *int      `json:"years_experience,omitempty"`
	HourlyRate      *int      `json:"hourly_rate,omitempty"`
	Availability    *string   `json:"availability,omitempty"`
	PortfolioURL    *string   `json:"portfolio_url,omitempty"`
	DemoReelURL     *string   `json:"demo_reel_url,omitempty"`
}

type AddPortfolioItemRequest struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	ThumbnailURL string `json:"thumbnail_url"`
	MediaURL     string `json:"media_url"`
	MediaType    string `json:"media_type"`
	Platform     string `json:"platform"`
}

type SkillInput struct {
	Name        string `json:"name"`
	Proficiency string `json:"proficiency"`
}

type SetSkillsRequest struct {
	Skills []SkillInput `json:"skills"`
}

type CreateMarketplaceTemplateRequest struct {
	Title         string `json:"title"`
	Description   string `json:"description"`
	Category      string `json:"category"`
	BriefTemplate string `json:"brief_template"`
	Price         int    `json:"price"`
}

type EditorSearchFilter struct {
	Skills       []string `json:"skills"`
	MinRating    float64  `json:"min_rating"`
	MaxRate      int      `json:"max_rate"`
	Availability string   `json:"availability"`
	Page         int      `json:"page"`
	PageSize     int      `json:"page_size"`
}

type EditorSearchResult struct {
	EditorID      uuid.UUID     `json:"editor_id"`
	Name          string        `json:"name"`
	AvatarURL     string        `json:"avatar_url"`
	Bio           string        `json:"bio"`
	Skills        []EditorSkill `json:"skills"`
	AvgRating     float64       `json:"avg_rating"`
	CompletedJobs int           `json:"completed_jobs"`
	HourlyRate    int           `json:"hourly_rate"`
	Availability  string        `json:"availability"`
	MatchScore    int           `json:"match_score"`
}
