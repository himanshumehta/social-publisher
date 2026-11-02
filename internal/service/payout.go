package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PayoutService struct {
	pool *pgxpool.Pool
}

func NewPayoutService(pool *pgxpool.Pool) *PayoutService {
	return &PayoutService{pool: pool}
}

// --- Response types ---

type PayoutSummary struct {
	ID          uuid.UUID `json:"id"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	GrossAmount int64     `json:"gross_amount"`
	PlatformFee int64     `json:"platform_fee"`
	TDSAmount   int64     `json:"tds_amount"`
	NetAmount   int64     `json:"net_amount"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type CreatorPayoutResponse struct {
	BankingStatus string          `json:"banking_status"`
	TotalEarned   int64           `json:"total_earned"`
	TotalPending  int64           `json:"total_pending"`
	TotalPaid     int64           `json:"total_paid"`
	Payouts       []PayoutSummary `json:"payouts"`
}

type PayoutWithCreator struct {
	ID          uuid.UUID `json:"id"`
	CreatorID   uuid.UUID `json:"creator_id"`
	CreatorName string    `json:"creator_name"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	GrossAmount int64     `json:"gross_amount"`
	PlatformFee int64     `json:"platform_fee"`
	TDSAmount   int64     `json:"tds_amount"`
	NetAmount   int64     `json:"net_amount"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *PayoutService) GetCreatorPayouts(ctx context.Context, creatorID uuid.UUID) (*CreatorPayoutResponse, error) {
	resp := &CreatorPayoutResponse{}

	// Get banking status
	var bankingStatus *string
	err := s.pool.QueryRow(ctx, `SELECT status::text FROM banking_info WHERE user_id = $1`, creatorID).Scan(&bankingStatus)
	if err != nil {
		resp.BankingStatus = "not_submitted"
	} else if bankingStatus != nil {
		resp.BankingStatus = *bankingStatus
	}

	// Get totals
	err = s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(gross_amount), 0),
			COALESCE(SUM(CASE WHEN status IN ('pending', 'approved', 'processing') THEN net_amount ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'completed' THEN net_amount ELSE 0 END), 0)
		FROM payouts WHERE creator_id = $1
	`, creatorID).Scan(&resp.TotalEarned, &resp.TotalPending, &resp.TotalPaid)
	if err != nil {
		return nil, fmt.Errorf("get payout totals: %w", err)
	}

	// Get payout list
	rows, err := s.pool.Query(ctx, `
		SELECT id, period_start, period_end, gross_amount, platform_fee, tds_amount, net_amount, status::text, created_at
		FROM payouts WHERE creator_id = $1
		ORDER BY period_end DESC
	`, creatorID)
	if err != nil {
		return nil, fmt.Errorf("list creator payouts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p PayoutSummary
		if err := rows.Scan(&p.ID, &p.PeriodStart, &p.PeriodEnd, &p.GrossAmount, &p.PlatformFee, &p.TDSAmount, &p.NetAmount, &p.Status, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan payout: %w", err)
		}
		resp.Payouts = append(resp.Payouts, p)
	}
	if resp.Payouts == nil {
		resp.Payouts = []PayoutSummary{}
	}

	return resp, nil
}

func (s *PayoutService) ListAllPayouts(ctx context.Context, status string, limit, offset int) ([]PayoutWithCreator, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var args []interface{}
	where := ""
	argIdx := 1

	if status != "" {
		where = fmt.Sprintf("WHERE p.status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	var total int
	countQ := "SELECT COUNT(*) FROM payouts p"
	if where != "" {
		countQ += " " + where
	}
	if err := s.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count payouts: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT p.id, p.creator_id, COALESCE(u.display_name, u.name), p.period_start, p.period_end,
		       p.gross_amount, p.platform_fee, p.tds_amount, p.net_amount, p.status::text, p.created_at
		FROM payouts p
		JOIN users u ON u.id = p.creator_id
		%s
		ORDER BY p.created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list all payouts: %w", err)
	}
	defer rows.Close()

	var results []PayoutWithCreator
	for rows.Next() {
		var p PayoutWithCreator
		if err := rows.Scan(&p.ID, &p.CreatorID, &p.CreatorName, &p.PeriodStart, &p.PeriodEnd, &p.GrossAmount, &p.PlatformFee, &p.TDSAmount, &p.NetAmount, &p.Status, &p.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan payout: %w", err)
		}
		results = append(results, p)
	}
	if results == nil {
		results = []PayoutWithCreator{}
	}

	return results, total, nil
}
