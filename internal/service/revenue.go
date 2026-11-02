package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RevenueService struct {
	pool *pgxpool.Pool
}

func NewRevenueService(pool *pgxpool.Pool) *RevenueService {
	return &RevenueService{pool: pool}
}

// RevenueConfig holds admin-configurable revenue sharing settings.
type RevenueConfig struct {
	CreatorSharePct int   `json:"creator_share_pct"` // e.g. 70
	TDSRatePct      int   `json:"tds_rate_pct"`      // e.g. 10
	MinPayoutAmount int64 `json:"min_payout_amount"` // paise, e.g. 50000 = Rs500
}

func (s *RevenueService) GetConfig(ctx context.Context) (*RevenueConfig, error) {
	cfg := &RevenueConfig{CreatorSharePct: 70, TDSRatePct: 10, MinPayoutAmount: 50000}
	rows, err := s.pool.Query(ctx, `SELECT config_key, config_value FROM revenue_config`)
	if err != nil {
		return cfg, nil // return defaults on error
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var val int64
		if err := rows.Scan(&key, &val); err != nil {
			continue
		}
		switch key {
		case "creator_share_pct":
			cfg.CreatorSharePct = int(val)
		case "tds_rate_pct":
			cfg.TDSRatePct = int(val)
		case "min_payout_amount":
			cfg.MinPayoutAmount = val
		}
	}
	return cfg, nil
}

func (s *RevenueService) UpdateConfig(ctx context.Context, adminID uuid.UUID, key string, value int64) error {
	validKeys := map[string]bool{"creator_share_pct": true, "tds_rate_pct": true, "min_payout_amount": true}
	if !validKeys[key] {
		return fmt.Errorf("invalid config key: %s", key)
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO revenue_config (config_key, config_value, updated_by, updated_at)
		 VALUES ($1, $2::jsonb, $3, NOW())
		 ON CONFLICT (config_key) DO UPDATE SET config_value = $2::jsonb, updated_by = $3, updated_at = NOW()`,
		key, fmt.Sprintf("%d", value), adminID)
	if err != nil {
		return fmt.Errorf("update revenue config: %w", err)
	}
	return nil
}

// CreatorMonthlyEarning represents aggregated earnings for one creator in a month.
type CreatorMonthlyEarning struct {
	CreatorID uuid.UUID
	Gross     int64 // total estimated_revenue_paise
}

// CalculateMonthlyPayouts generates payout records for all creators for a given month.
// It aggregates youtube_analytics data, applies revenue split, and inserts payout records.
func (s *RevenueService) CalculateMonthlyPayouts(ctx context.Context, year int, month time.Month, adminID uuid.UUID) (int, error) {
	periodStart := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0)

	cfg, err := s.GetConfig(ctx)
	if err != nil {
		return 0, fmt.Errorf("get revenue config: %w", err)
	}

	// Aggregate per-creator revenue for the month
	rows, err := s.pool.Query(ctx, `
		SELECT p.creator_id, COALESCE(SUM(ya.estimated_revenue_paise), 0) AS gross
		FROM youtube_analytics ya
		JOIN publish_records pr ON ya.publish_record_id = pr.id
		JOIN projects p ON pr.project_id = p.id
		WHERE ya.period_start >= $1 AND ya.period_start < $2
		  AND p.creator_id IS NOT NULL
		GROUP BY p.creator_id
		HAVING SUM(ya.estimated_revenue_paise) > 0
	`, periodStart, periodEnd)
	if err != nil {
		return 0, fmt.Errorf("aggregate earnings: %w", err)
	}
	defer rows.Close()

	var earnings []CreatorMonthlyEarning
	for rows.Next() {
		var e CreatorMonthlyEarning
		if err := rows.Scan(&e.CreatorID, &e.Gross); err != nil {
			return 0, fmt.Errorf("scan earning: %w", err)
		}
		earnings = append(earnings, e)
	}

	created := 0
	for _, e := range earnings {
		creatorShare := e.Gross * int64(cfg.CreatorSharePct) / 100
		platformFee := e.Gross - creatorShare
		tds := creatorShare * int64(cfg.TDSRatePct) / 100
		net := creatorShare - tds

		if net < cfg.MinPayoutAmount {
			continue // skip if below minimum
		}

		_, err := s.pool.Exec(ctx, `
			INSERT INTO payouts (creator_id, period_start, period_end, gross_amount, platform_fee, tds_amount, net_amount, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending')
			ON CONFLICT (creator_id, period_start, period_end) DO NOTHING
		`, e.CreatorID, periodStart, periodEnd, e.Gross, platformFee, tds, net)
		if err != nil {
			return created, fmt.Errorf("insert payout for creator %s: %w", e.CreatorID, err)
		}
		created++
	}

	return created, nil
}

// SyncAnalytics fetches analytics for all published videos on a specific channel.
// This is the hook point for YouTube Analytics API integration.
func (s *RevenueService) SyncAnalytics(ctx context.Context, channelID uuid.UUID) error {
	// Get publish records for this channel that need sync
	rows, err := s.pool.Query(ctx, `
		SELECT pr.id, pr.platform_video_id
		FROM publish_records pr
		WHERE pr.channel_id = $1 AND pr.status = 'completed' AND pr.platform_video_id IS NOT NULL AND pr.platform_video_id != ''
	`, channelID)
	if err != nil {
		return fmt.Errorf("get publish records: %w", err)
	}
	defer rows.Close()

	type publishTarget struct {
		ID              uuid.UUID
		PlatformVideoID string
	}
	var targets []publishTarget
	for rows.Next() {
		var t publishTarget
		if err := rows.Scan(&t.ID, &t.PlatformVideoID); err != nil {
			return fmt.Errorf("scan publish record: %w", err)
		}
		targets = append(targets, t)
	}

	if len(targets) == 0 {
		return nil
	}

	// TODO: Replace with real YouTube Analytics API call.
	// The actual implementation would:
	// 1. Get the channel's OAuth access token (decrypt via TokenCrypto)
	// 2. Call YouTube Analytics API v2: GET https://youtubeanalytics.googleapis.com/v2/reports
	//    with metrics=estimatedRevenue,views,estimatedMinutesWatched,likes,shares,subscribersGained
	//    and dimensions=video, filters=video==VIDEO_ID1,VIDEO_ID2,...
	// 3. Parse response and upsert into youtube_analytics table
	// 4. Convert USD revenue to INR paise using exchange rate

	now := time.Now()
	_, err = s.pool.Exec(ctx,
		`UPDATE channels SET last_analytics_sync = $1, analytics_sync_error = '' WHERE id = $2`,
		now, channelID)
	if err != nil {
		return fmt.Errorf("update sync timestamp: %w", err)
	}

	return nil
}

// ApprovePayout marks a payout as approved by an admin.
func (s *RevenueService) ApprovePayout(ctx context.Context, payoutID, adminID uuid.UUID) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE payouts SET status = 'approved', approved_by = $1, approved_at = NOW(), updated_at = NOW()
		 WHERE id = $2 AND status = 'pending'`,
		adminID, payoutID)
	if err != nil {
		return fmt.Errorf("approve payout: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("payout not found or not in pending status")
	}
	return nil
}

// RejectPayout cancels a payout with a reason.
func (s *RevenueService) RejectPayout(ctx context.Context, payoutID, adminID uuid.UUID, reason string) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE payouts SET status = 'cancelled', failure_reason = $1, updated_at = NOW()
		 WHERE id = $2 AND status = 'pending'`,
		reason, payoutID)
	if err != nil {
		return fmt.Errorf("reject payout: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("payout not found or not in pending status")
	}
	return nil
}

// GenerateMonthlyPayouts is called by admin to trigger payout generation for previous month.
func (s *RevenueService) GenerateMonthlyPayouts(ctx context.Context, adminID uuid.UUID) (int, error) {
	now := time.Now()
	prevMonth := now.AddDate(0, -1, 0)
	return s.CalculateMonthlyPayouts(ctx, prevMonth.Year(), prevMonth.Month(), adminID)
}
