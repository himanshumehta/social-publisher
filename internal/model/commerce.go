package model

import (
	"time"

	"github.com/google/uuid"
)

// ── Platform & type enums ─────────────────────────────────────────────

type CommercePlatform string

const (
	PlatformYouTube   CommercePlatform = "youtube"
	PlatformInstagram CommercePlatform = "instagram"
	PlatformShopify   CommercePlatform = "shopify"
	PlatformGumroad   CommercePlatform = "gumroad"
	PlatformPatreon   CommercePlatform = "patreon"
)

type IntegrationType string

const (
	IntegrationSuperChat      IntegrationType = "super_chat"
	IntegrationMembership     IntegrationType = "membership"
	IntegrationShopping       IntegrationType = "shopping"
	IntegrationDigitalProduct IntegrationType = "digital_product"
	IntegrationSubscription   IntegrationType = "subscription"
)

type TransactionType string

const (
	TxnSuperChat           TransactionType = "super_chat"
	TxnMembershipPayment   TransactionType = "membership_payment"
	TxnProductSale         TransactionType = "product_sale"
	TxnSubscriptionPayment TransactionType = "subscription_payment"
	TxnTip                 TransactionType = "tip"
	TxnDonation            TransactionType = "donation"
)

// ── Entities ──────────────────────────────────────────────────────────

type CommerceIntegration struct {
	ID              uuid.UUID         `json:"id"`
	UserID          uuid.UUID         `json:"user_id"`
	ChannelID       *uuid.UUID        `json:"channel_id,omitempty"`
	Platform        string            `json:"platform"`
	IntegrationType string            `json:"integration_type"`
	Credentials     map[string]string `json:"-"` // never exposed in JSON
	Status          string            `json:"status"`
	LastSyncAt      *time.Time        `json:"last_sync_at,omitempty"`
	SyncError       *string           `json:"sync_error,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

type CommerceTransaction struct {
	ID                    uuid.UUID      `json:"id"`
	IntegrationID         uuid.UUID      `json:"integration_id"`
	UserID                uuid.UUID      `json:"user_id"`
	Platform              string         `json:"platform"`
	TransactionType       string         `json:"transaction_type"`
	ExternalTransactionID *string        `json:"external_transaction_id,omitempty"`
	AmountPaise           int64          `json:"amount_paise"`
	Currency              string         `json:"currency"`
	CustomerName          *string        `json:"customer_name,omitempty"`
	CustomerID            *string        `json:"customer_id,omitempty"`
	ProductName           *string        `json:"product_name,omitempty"`
	Metadata              map[string]any `json:"metadata,omitempty"`
	TransactionDate       time.Time      `json:"transaction_date"`
	CreatedAt             time.Time      `json:"created_at"`
}

// ── DTOs ──────────────────────────────────────────────────────────────

type ConnectCommerceRequest struct {
	Platform        string            `json:"platform"`
	IntegrationType string            `json:"integration_type"`
	ChannelID       *uuid.UUID        `json:"channel_id,omitempty"`
	Credentials     map[string]string `json:"credentials"`
}

type CommerceStats struct {
	TotalRevenuePaise int64                       `json:"total_revenue_paise"`
	TransactionCount  int                         `json:"transaction_count"`
	ByPlatform        map[string]PlatformCommerce `json:"by_platform"`
	ByType            map[string]int64            `json:"by_type"`
	TopProducts       []ProductStat               `json:"top_products"`
	PeriodStart       time.Time                   `json:"period_start"`
	PeriodEnd         time.Time                   `json:"period_end"`
}

type PlatformCommerce struct {
	Platform         string     `json:"platform"`
	RevenuePaise     int64      `json:"revenue_paise"`
	TransactionCount int        `json:"transaction_count"`
	LastSyncAt       *time.Time `json:"last_sync_at,omitempty"`
	Status           string     `json:"status"`
}

type ProductStat struct {
	Name         string `json:"name"`
	RevenuePaise int64  `json:"revenue_paise"`
	Count        int    `json:"count"`
}

type TransactionFilter struct {
	Platform        string    `json:"platform"`
	TransactionType string    `json:"transaction_type"`
	StartDate       time.Time `json:"start_date"`
	EndDate         time.Time `json:"end_date"`
	Page            int       `json:"page"`
	PageSize        int       `json:"page_size"`
}
