package model

import (
	"time"

	"github.com/google/uuid"
)

type PayoutStatus string

const (
	PayoutPending    PayoutStatus = "pending"
	PayoutApproved   PayoutStatus = "approved"
	PayoutProcessing PayoutStatus = "processing"
	PayoutCompleted  PayoutStatus = "completed"
	PayoutFailed     PayoutStatus = "failed"
	PayoutCancelled  PayoutStatus = "cancelled"
)

type Payout struct {
	ID                uuid.UUID    `json:"id"`
	CreatorID         uuid.UUID    `json:"creator_id"`
	PeriodStart       time.Time    `json:"period_start"`
	PeriodEnd         time.Time    `json:"period_end"`
	GrossAmount       int64        `json:"gross_amount"`
	PlatformFee       int64        `json:"platform_fee"`
	TDSAmount         int64        `json:"tds_amount"`
	NetAmount         int64        `json:"net_amount"`
	Status            PayoutStatus `json:"status"`
	ApprovedBy        *uuid.UUID   `json:"approved_by,omitempty"`
	ApprovedAt        *time.Time   `json:"approved_at,omitempty"`
	TransferMode      string       `json:"transfer_mode,omitempty"`
	UPITransactionRef string       `json:"upi_transaction_ref,omitempty"`
	ProcessedBy       *uuid.UUID   `json:"processed_by,omitempty"`
	RazorpayPayoutID  string       `json:"razorpay_payout_id,omitempty"`
	RazorpayFundAccID string       `json:"razorpay_fund_acc_id,omitempty"`
	RazorpayStatus    string       `json:"razorpay_status,omitempty"`
	FailureReason     string       `json:"failure_reason,omitempty"`
	Breakdown         []byte       `json:"breakdown"`
	ProcessedAt       *time.Time   `json:"processed_at,omitempty"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}
