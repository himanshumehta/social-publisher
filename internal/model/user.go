package model

import (
	"time"

	"github.com/google/uuid"
)

type UserRole string

const (
	RoleSuperAdmin UserRole = "super_admin"
	RoleAdmin      UserRole = "admin"
	RoleEditor     UserRole = "editor"
	RoleCreator    UserRole = "creator"
)

type OAuthProvider string

const (
	ProviderGoogle   OAuthProvider = "google"
	ProviderFacebook OAuthProvider = "facebook"
	ProviderLinkedIn OAuthProvider = "linkedin"
)

type BankingStatus string

const (
	BankingPending   BankingStatus = "pending"
	BankingSubmitted BankingStatus = "submitted"
	BankingVerified  BankingStatus = "verified"
	BankingRejected  BankingStatus = "rejected"
)

type User struct {
	ID          uuid.UUID  `json:"id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	DisplayName string     `json:"display_name,omitempty"`
	AvatarURL   string     `json:"avatar_url,omitempty"`
	Bio         string     `json:"bio,omitempty"`
	Phone       string     `json:"phone,omitempty"`
	Role        UserRole   `json:"role"`
	IsActive    bool       `json:"is_active"`
	IsOnboarded bool       `json:"is_onboarded"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type OAuthAccount struct {
	ID             uuid.UUID     `json:"id"`
	UserID         uuid.UUID     `json:"user_id"`
	Provider       OAuthProvider `json:"provider"`
	ProviderID     string        `json:"provider_id"`
	AccessToken    string        `json:"-"`
	RefreshToken   string        `json:"-"`
	TokenExpiresAt *time.Time    `json:"token_expires_at,omitempty"`
	RawProfile     []byte        `json:"-"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type RefreshToken struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	TokenHash string    `json:"-"`
	FamilyID  uuid.UUID `json:"family_id"`
	IsRevoked bool      `json:"is_revoked"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type CreatorProfile struct {
	ID                 uuid.UUID `json:"id"`
	UserID             uuid.UUID `json:"user_id"`
	Slug               string    `json:"slug"`
	City               string    `json:"city,omitempty"`
	State              string    `json:"state,omitempty"`
	Specializations    []string  `json:"specializations,omitempty"`
	YouTubeChannel     string    `json:"youtube_channel,omitempty"`
	InstagramHandle    string    `json:"instagram_handle,omitempty"`
	FacebookPageURL    string    `json:"facebook_page_url,omitempty"`
	TwitterHandle      string    `json:"twitter_handle,omitempty"`
	LinkedInProfileURL string    `json:"linkedin_profile_url,omitempty"`
	ThreadsHandle      string    `json:"threads_handle,omitempty"`
	TotalVideos        int       `json:"total_videos"`
	TotalViews         int64     `json:"total_views"`
	TotalEarnings      int64     `json:"total_earnings"` // paise
	IsPublic           bool      `json:"is_public"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type BankingInfo struct {
	ID              uuid.UUID     `json:"id"`
	UserID          uuid.UUID     `json:"user_id"`
	AccountHolder   string        `json:"account_holder"`
	BankName        string        `json:"bank_name"`
	AccountNumber   string        `json:"-"` // encrypted, never in JSON
	IFSCCode        string        `json:"ifsc_code"`
	UPIID           string        `json:"upi_id,omitempty"`
	PANNumber       string        `json:"-"` // encrypted
	Status          BankingStatus `json:"status"`
	VerifiedAt      *time.Time    `json:"verified_at,omitempty"`
	VerifiedBy      *uuid.UUID    `json:"verified_by,omitempty"`
	RejectionReason string        `json:"rejection_reason,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}
