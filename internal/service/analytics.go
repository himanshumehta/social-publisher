package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// AnalyticsService handles all cross-platform analytics operations.
type AnalyticsService struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewAnalyticsService constructs the service.
func NewAnalyticsService(pool *pgxpool.Pool) *AnalyticsService {
	return &AnalyticsService{
		pool:   pool,
		logger: slog.Default().With("service", "analytics"),
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// calculateEngagementRate returns (likes+comments+shares)/views * 100.
// Returns 0 when views is zero to avoid division by zero.
func calculateEngagementRate(likes, comments, shares, views int) float64 {
	if views == 0 {
		return 0
	}
	return float64(likes+comments+shares) / float64(views) * 100
}

// growthPct computes percentage growth from prev to curr.
// If prev is zero, returns 100 (represents new/infinite growth).
func growthPct(curr, prev float64) float64 {
	if prev == 0 {
		if curr == 0 {
			return 0
		}
		return 100
	}
	return (curr - prev) / prev * 100
}

// ---------------------------------------------------------------------------
// GetOverview
// ---------------------------------------------------------------------------

// GetOverview returns an aggregated overview across all platforms for a user
// within the given date range, including growth vs the immediately preceding
// period of equal length.
func (s *AnalyticsService) GetOverview(
	ctx context.Context,
	userID uuid.UUID,
	dr model.DateRange,
) (*model.AnalyticsOverview, error) {

	// --- current period platform breakdown ---
	rows, err := s.pool.Query(ctx, `
		SELECT
			platform,
			COALESCE(MAX(followers), 0)             AS followers,
			COALESCE(SUM(views), 0)                  AS views,
			COALESCE(SUM(likes), 0)                  AS likes,
			COALESCE(SUM(comments), 0)               AS comments,
			COALESCE(SUM(shares), 0)                 AS shares,
			COALESCE(SUM(revenue_paise), 0)          AS revenue_paise
		FROM analytics_snapshots
		WHERE user_id = $1
		  AND snapshot_date >= $2
		  AND snapshot_date <  $3
		GROUP BY platform
		ORDER BY platform
	`, userID, dr.Start, dr.End)
	if err != nil {
		return nil, fmt.Errorf("analytics overview query: %w", err)
	}
	defer rows.Close()

	var breakdown []model.PlatformStat
	var totalFollowers, totalViews, totalEngagements int

	for rows.Next() {
		var ps model.PlatformStat
		if err := rows.Scan(
			&ps.Platform,
			&ps.Followers,
			&ps.Views,
			&ps.Likes,
			&ps.Comments,
			&ps.Shares,
			&ps.Revenue,
		); err != nil {
			return nil, fmt.Errorf("scan platform stat: %w", err)
		}
		ps.EngagementRate = calculateEngagementRate(ps.Likes, ps.Comments, ps.Shares, ps.Views)
		totalFollowers += ps.Followers
		totalViews += ps.Views
		totalEngagements += ps.Likes + ps.Comments + ps.Shares
		breakdown = append(breakdown, ps)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	// --- previous period (same length, immediately before) ---
	duration := dr.End.Sub(dr.Start)
	prevRange := model.DateRange{
		Start: dr.Start.Add(-duration),
		End:   dr.Start,
	}

	var prevFollowers, prevViews, prevEngagements int
	err = s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(MAX(followers), 0),
			COALESCE(SUM(views), 0),
			COALESCE(SUM(likes) + SUM(comments) + SUM(shares), 0)
		FROM analytics_snapshots
		WHERE user_id = $1
		  AND snapshot_date >= $2
		  AND snapshot_date <  $3
	`, userID, prevRange.Start, prevRange.End).Scan(
		&prevFollowers, &prevViews, &prevEngagements,
	)
	if err != nil {
		return nil, fmt.Errorf("previous period query: %w", err)
	}

	overview := &model.AnalyticsOverview{
		TotalFollowers:   totalFollowers,
		TotalViews:       totalViews,
		TotalEngagements: totalEngagements,
		EngagementRate:   calculateEngagementRate(totalEngagements, 0, 0, totalViews),
		PeriodGrowth: model.PeriodGrowth{
			Followers:   growthPct(float64(totalFollowers), float64(prevFollowers)),
			Views:       growthPct(float64(totalViews), float64(prevViews)),
			Engagements: growthPct(float64(totalEngagements), float64(prevEngagements)),
		},
		PlatformBreakdown: breakdown,
	}
	// engagement rate for the overview uses the combined totals
	overview.EngagementRate = calculateEngagementRate(totalEngagements, 0, 0, totalViews)

	return overview, nil
}

// ---------------------------------------------------------------------------
// GetPlatformStats
// ---------------------------------------------------------------------------

// GetPlatformStats returns daily snapshots for a specific platform within a
// date range, ordered by date ascending.
func (s *AnalyticsService) GetPlatformStats(
	ctx context.Context,
	userID uuid.UUID,
	platform string,
	dr model.DateRange,
) ([]model.AnalyticsSnapshot, error) {

	rows, err := s.pool.Query(ctx, `
		SELECT
			id, user_id, channel_id, platform, snapshot_date,
			followers, views, likes, comments, shares,
			engagement_rate, impressions, reach, clicks,
			revenue_paise, metadata, created_at
		FROM analytics_snapshots
		WHERE user_id  = $1
		  AND platform = $2
		  AND snapshot_date >= $3
		  AND snapshot_date <  $4
		ORDER BY snapshot_date ASC
	`, userID, platform, dr.Start, dr.End)
	if err != nil {
		return nil, fmt.Errorf("platform stats query: %w", err)
	}
	defer rows.Close()

	var snapshots []model.AnalyticsSnapshot
	for rows.Next() {
		var snap model.AnalyticsSnapshot
		if err := rows.Scan(
			&snap.ID, &snap.UserID, &snap.ChannelID, &snap.Platform,
			&snap.SnapshotDate, &snap.Followers, &snap.Views,
			&snap.Likes, &snap.Comments, &snap.Shares,
			&snap.EngagementRate, &snap.Impressions, &snap.Reach,
			&snap.Clicks, &snap.RevenuePaise, &snap.Metadata,
			&snap.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}
		snapshots = append(snapshots, snap)
	}

	return snapshots, rows.Err()
}

// ---------------------------------------------------------------------------
// GetPostAnalytics
// ---------------------------------------------------------------------------

// GetPostAnalytics returns per-platform metrics for a given post.
// It joins publish_records (or the posts table) to correlate a post with its
// analytics snapshots. The metadata JSONB is expected to contain a
// "post_id" key when the snapshot was generated from a published post.
func (s *AnalyticsService) GetPostAnalytics(
	ctx context.Context,
	postID uuid.UUID,
) ([]model.PostAnalytics, error) {

	rows, err := s.pool.Query(ctx, `
		SELECT
			$1::uuid                              AS post_id,
			a.platform,
			COALESCE(SUM(a.views), 0)              AS views,
			COALESCE(SUM(a.likes), 0)              AS likes,
			COALESCE(SUM(a.comments), 0)           AS comments,
			COALESCE(SUM(a.shares), 0)             AS shares,
			COALESCE(SUM(a.clicks), 0)             AS clicks,
			0::NUMERIC                             AS engagement_rate
		FROM analytics_snapshots a
		WHERE a.metadata->>'post_id' = $1::text
		GROUP BY a.platform
		ORDER BY a.platform
	`, postID)
	if err != nil {
		return nil, fmt.Errorf("post analytics query: %w", err)
	}
	defer rows.Close()

	var results []model.PostAnalytics
	for rows.Next() {
		var pa model.PostAnalytics
		if err := rows.Scan(
			&pa.PostID, &pa.Platform,
			&pa.Views, &pa.Likes, &pa.Comments,
			&pa.Shares, &pa.Clicks, &pa.EngagementRate,
		); err != nil {
			return nil, fmt.Errorf("scan post analytics: %w", err)
		}
		pa.EngagementRate = calculateEngagementRate(pa.Likes, pa.Comments, pa.Shares, pa.Views)
		results = append(results, pa)
	}

	return results, rows.Err()
}

// ---------------------------------------------------------------------------
// GetComparison
// ---------------------------------------------------------------------------

// GetComparison returns aggregated metrics for two arbitrary periods so the
// caller can display period-over-period changes.
func (s *AnalyticsService) GetComparison(
	ctx context.Context,
	userID uuid.UUID,
	current, previous model.DateRange,
) (*model.AnalyticsComparison, error) {

	aggregate := func(dr model.DateRange) (model.AggregatedMetrics, error) {
		var m model.AggregatedMetrics
		err := s.pool.QueryRow(ctx, `
			SELECT
				COALESCE(MAX(followers), 0),
				COALESCE(SUM(views), 0),
				COALESCE(SUM(likes), 0),
				COALESCE(SUM(comments), 0),
				COALESCE(SUM(shares), 0),
				COALESCE(SUM(revenue_paise), 0)
			FROM analytics_snapshots
			WHERE user_id = $1
			  AND snapshot_date >= $2
			  AND snapshot_date <  $3
		`, userID, dr.Start, dr.End).Scan(
			&m.Followers, &m.Views, &m.Likes,
			&m.Comments, &m.Shares, &m.Revenue,
		)
		if err != nil && err != pgx.ErrNoRows {
			return m, fmt.Errorf("aggregate query: %w", err)
		}
		m.Engagements = m.Likes + m.Comments + m.Shares
		m.EngRate = calculateEngagementRate(m.Engagements, 0, 0, m.Views)
		return m, nil
	}

	curr, err := aggregate(current)
	if err != nil {
		return nil, err
	}
	prev, err := aggregate(previous)
	if err != nil {
		return nil, err
	}

	return &model.AnalyticsComparison{
		CurrentPeriod:  curr,
		PreviousPeriod: prev,
	}, nil
}

// ---------------------------------------------------------------------------
// SaveSnapshot (upsert)
// ---------------------------------------------------------------------------

// SaveSnapshot inserts or updates a daily analytics snapshot.
// On conflict (same channel + platform + date) it updates all metric columns.
func (s *AnalyticsService) SaveSnapshot(
	ctx context.Context,
	snap *model.AnalyticsSnapshot,
) error {

	snap.EngagementRate = calculateEngagementRate(
		snap.Likes, snap.Comments, snap.Shares, snap.Views,
	)

	_, err := s.pool.Exec(ctx, `
		INSERT INTO analytics_snapshots (
			user_id, channel_id, platform, snapshot_date,
			followers, views, likes, comments, shares,
			engagement_rate, impressions, reach, clicks,
			revenue_paise, metadata
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8, $9,
			$10, $11, $12, $13,
			$14, $15
		)
		ON CONFLICT (channel_id, platform, snapshot_date)
		DO UPDATE SET
			followers       = EXCLUDED.followers,
			views           = EXCLUDED.views,
			likes           = EXCLUDED.likes,
			comments        = EXCLUDED.comments,
			shares          = EXCLUDED.shares,
			engagement_rate = EXCLUDED.engagement_rate,
			impressions     = EXCLUDED.impressions,
			reach           = EXCLUDED.reach,
			clicks          = EXCLUDED.clicks,
			revenue_paise   = EXCLUDED.revenue_paise,
			metadata        = EXCLUDED.metadata
	`, snap.UserID, snap.ChannelID, snap.Platform, snap.SnapshotDate,
		snap.Followers, snap.Views, snap.Likes, snap.Comments, snap.Shares,
		snap.EngagementRate, snap.Impressions, snap.Reach, snap.Clicks,
		snap.RevenuePaise, snap.Metadata,
	)
	if err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// PullPlatformAnalytics
// ---------------------------------------------------------------------------

// PullPlatformAnalytics fetches analytics from the given platform's API and
// stores the result as a daily snapshot. Currently stubbed per platform.
func (s *AnalyticsService) PullPlatformAnalytics(
	ctx context.Context,
	channelID uuid.UUID,
	platform string,
) error {

	switch platform {
	case "youtube":
		s.logger.Info("platform pull not yet implemented", "platform", "youtube", "channel_id", channelID)
	case "instagram":
		s.logger.Info("platform pull not yet implemented", "platform", "instagram", "channel_id", channelID)
	case "tiktok":
		s.logger.Info("platform pull not yet implemented", "platform", "tiktok", "channel_id", channelID)
	case "twitter":
		s.logger.Info("platform pull not yet implemented", "platform", "twitter", "channel_id", channelID)
	default:
		s.logger.Warn("unknown platform for analytics pull", "platform", platform, "channel_id", channelID)
	}

	return nil
}

// ---------------------------------------------------------------------------
// AggregateDaily
// ---------------------------------------------------------------------------

// AggregateDaily recalculates engagement_rate for all snapshots that were
// recorded today. This is safe to call repeatedly (idempotent).
func (s *AnalyticsService) AggregateDaily(ctx context.Context) error {
	today := time.Now().UTC().Truncate(24 * time.Hour)

	_, err := s.pool.Exec(ctx, `
		UPDATE analytics_snapshots
		SET engagement_rate = CASE
			WHEN views > 0
			THEN ROUND((likes + comments + shares)::NUMERIC / views * 100, 4)
			ELSE 0
		END
		WHERE snapshot_date = $1
	`, today)
	if err != nil {
		return fmt.Errorf("aggregate daily: %w", err)
	}

	s.logger.Info("daily aggregation complete", "date", today.Format("2006-01-02"))
	return nil
}
