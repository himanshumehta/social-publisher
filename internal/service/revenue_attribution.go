package service

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// RevenueAttributionService handles revenue tracking, goals, and affiliate links.
type RevenueAttributionService struct {
	pool *pgxpool.Pool
}

// NewRevenueAttributionService creates a new RevenueAttributionService.
func NewRevenueAttributionService(pool *pgxpool.Pool) *RevenueAttributionService {
	return &RevenueAttributionService{pool: pool}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// calculatePeriodDates returns the start and end dates for the current instance
// of the given period type (weekly, monthly, quarterly, yearly).
func calculatePeriodDates(period string) (time.Time, time.Time) {
	now := time.Now().UTC()

	switch period {
	case "weekly":
		// Week starts on Monday
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7 // Sunday = 7
		}
		start := now.AddDate(0, 0, -(weekday - 1))
		start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 0, 7)
		return start, end

	case "monthly":
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, 0)
		return start, end

	case "quarterly":
		quarter := (int(now.Month()) - 1) / 3
		startMonth := time.Month(quarter*3 + 1)
		start := time.Date(now.Year(), startMonth, 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 3, 0)
		return start, end

	case "yearly":
		start := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(1, 0, 0)
		return start, end

	default:
		// Default to monthly
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, 0)
		return start, end
	}
}

// calculateGrowth returns the percentage growth from previous to current.
// Returns 0 when both are zero, 100 when previous is zero but current is not.
func calculateGrowth(current, previous int64) float64 {
	if previous == 0 {
		if current == 0 {
			return 0
		}
		return 100
	}
	return float64(current-previous) / float64(previous) * 100
}

// generateTrackingCode returns a short unique code (8 characters from a UUID).
func generateTrackingCode() string {
	id := uuid.New().String()
	// Remove hyphens and take first 8 characters
	clean := strings.ReplaceAll(id, "-", "")
	return clean[:8]
}

// ---------------------------------------------------------------------------
// Revenue Sources
// ---------------------------------------------------------------------------

// AddSource validates the source type and creates a new revenue source record.
func (s *RevenueAttributionService) AddSource(
	ctx context.Context,
	userID uuid.UUID,
	req *model.CreateRevenueSourceRequest,
) (*model.RevenueSource, error) {

	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if !model.ValidSourceTypes[req.SourceType] {
		return nil, fmt.Errorf("invalid source_type: %s", req.SourceType)
	}
	if req.AmountPaise < 0 {
		return nil, fmt.Errorf("amount_paise must be non-negative")
	}

	periodStart, err := time.Parse("2006-01-02", req.PeriodStart)
	if err != nil {
		return nil, fmt.Errorf("invalid period_start: %w", err)
	}
	periodEnd, err := time.Parse("2006-01-02", req.PeriodEnd)
	if err != nil {
		return nil, fmt.Errorf("invalid period_end: %w", err)
	}
	if !periodEnd.After(periodStart) {
		return nil, fmt.Errorf("period_end must be after period_start")
	}

	currency := req.Currency
	if currency == "" {
		currency = "INR"
	}

	src := &model.RevenueSource{}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO revenue_sources (
			user_id, name, source_type, platform, amount_paise,
			currency, period_start, period_end, notes
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, user_id, name, source_type, platform, amount_paise,
			currency, period_start, period_end, notes, metadata,
			created_at, updated_at`,
		userID, req.Name, string(req.SourceType), req.Platform,
		req.AmountPaise, currency, periodStart, periodEnd, req.Notes,
	).Scan(
		&src.ID, &src.UserID, &src.Name, &src.SourceType,
		&src.Platform, &src.AmountPaise, &src.Currency,
		&src.PeriodStart, &src.PeriodEnd, &src.Notes, &src.Metadata,
		&src.CreatedAt, &src.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert revenue source: %w", err)
	}

	return src, nil
}

// ListSources returns revenue sources for a user within the given date range.
func (s *RevenueAttributionService) ListSources(
	ctx context.Context,
	userID uuid.UUID,
	start, end time.Time,
) ([]model.RevenueSource, error) {

	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, source_type, platform, amount_paise,
			currency, period_start, period_end, notes, metadata,
			created_at, updated_at
		FROM revenue_sources
		WHERE user_id = $1
		  AND period_start >= $2
		  AND period_end <= $3
		ORDER BY period_start DESC, created_at DESC`,
		userID, start, end,
	)
	if err != nil {
		return nil, fmt.Errorf("query revenue sources: %w", err)
	}
	defer rows.Close()

	var sources []model.RevenueSource
	for rows.Next() {
		var src model.RevenueSource
		if err := rows.Scan(
			&src.ID, &src.UserID, &src.Name, &src.SourceType,
			&src.Platform, &src.AmountPaise, &src.Currency,
			&src.PeriodStart, &src.PeriodEnd, &src.Notes, &src.Metadata,
			&src.CreatedAt, &src.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan revenue source: %w", err)
		}
		sources = append(sources, src)
	}

	return sources, rows.Err()
}

// UpdateSource updates an existing revenue source.
func (s *RevenueAttributionService) UpdateSource(
	ctx context.Context,
	userID, sourceID uuid.UUID,
	req *model.CreateRevenueSourceRequest,
) (*model.RevenueSource, error) {

	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if !model.ValidSourceTypes[req.SourceType] {
		return nil, fmt.Errorf("invalid source_type: %s", req.SourceType)
	}

	periodStart, err := time.Parse("2006-01-02", req.PeriodStart)
	if err != nil {
		return nil, fmt.Errorf("invalid period_start: %w", err)
	}
	periodEnd, err := time.Parse("2006-01-02", req.PeriodEnd)
	if err != nil {
		return nil, fmt.Errorf("invalid period_end: %w", err)
	}
	if !periodEnd.After(periodStart) {
		return nil, fmt.Errorf("period_end must be after period_start")
	}

	currency := req.Currency
	if currency == "" {
		currency = "INR"
	}

	src := &model.RevenueSource{}
	err = s.pool.QueryRow(ctx, `
		UPDATE revenue_sources
		SET name = $3, source_type = $4, platform = $5,
			amount_paise = $6, currency = $7,
			period_start = $8, period_end = $9, notes = $10,
			updated_at = NOW()
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, name, source_type, platform, amount_paise,
			currency, period_start, period_end, notes, metadata,
			created_at, updated_at`,
		sourceID, userID, req.Name, string(req.SourceType), req.Platform,
		req.AmountPaise, currency, periodStart, periodEnd, req.Notes,
	).Scan(
		&src.ID, &src.UserID, &src.Name, &src.SourceType,
		&src.Platform, &src.AmountPaise, &src.Currency,
		&src.PeriodStart, &src.PeriodEnd, &src.Notes, &src.Metadata,
		&src.CreatedAt, &src.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("revenue source not found")
		}
		return nil, fmt.Errorf("update revenue source: %w", err)
	}

	return src, nil
}

// DeleteSource removes a revenue source owned by the given user.
func (s *RevenueAttributionService) DeleteSource(
	ctx context.Context,
	userID, sourceID uuid.UUID,
) error {

	tag, err := s.pool.Exec(ctx, `
		DELETE FROM revenue_sources
		WHERE id = $1 AND user_id = $2`,
		sourceID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete revenue source: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("revenue source not found")
	}

	return nil
}

// ---------------------------------------------------------------------------
// Revenue Breakdown
// ---------------------------------------------------------------------------

// GetBreakdown aggregates revenue by source_type and platform for a date range,
// and calculates growth vs the previous period of the same length.
func (s *RevenueAttributionService) GetBreakdown(
	ctx context.Context,
	userID uuid.UUID,
	start, end time.Time,
) (*model.RevenueBreakdown, error) {

	// --- aggregate by source_type ---
	bySource := make(map[string]int64)
	rows, err := s.pool.Query(ctx, `
		SELECT source_type, COALESCE(SUM(amount_paise), 0)
		FROM revenue_sources
		WHERE user_id = $1
		  AND period_start >= $2
		  AND period_end <= $3
		GROUP BY source_type`,
		userID, start, end,
	)
	if err != nil {
		return nil, fmt.Errorf("query breakdown by source: %w", err)
	}
	defer rows.Close()

	var totalPaise int64
	for rows.Next() {
		var sourceType string
		var amount int64
		if err := rows.Scan(&sourceType, &amount); err != nil {
			return nil, fmt.Errorf("scan source breakdown: %w", err)
		}
		bySource[sourceType] = amount
		totalPaise += amount
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	// --- aggregate by platform ---
	byPlatform := make(map[string]int64)
	rows2, err := s.pool.Query(ctx, `
		SELECT COALESCE(platform, 'direct'), COALESCE(SUM(amount_paise), 0)
		FROM revenue_sources
		WHERE user_id = $1
		  AND period_start >= $2
		  AND period_end <= $3
		GROUP BY platform`,
		userID, start, end,
	)
	if err != nil {
		return nil, fmt.Errorf("query breakdown by platform: %w", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var platform string
		var amount int64
		if err := rows2.Scan(&platform, &amount); err != nil {
			return nil, fmt.Errorf("scan platform breakdown: %w", err)
		}
		byPlatform[platform] = amount
	}
	if rows2.Err() != nil {
		return nil, rows2.Err()
	}

	// --- previous period for growth ---
	duration := end.Sub(start)
	prevStart := start.Add(-duration)
	prevEnd := start

	var prevTotal int64
	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_paise), 0)
		FROM revenue_sources
		WHERE user_id = $1
		  AND period_start >= $2
		  AND period_end <= $3`,
		userID, prevStart, prevEnd,
	).Scan(&prevTotal)
	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("query previous period total: %w", err)
	}

	return &model.RevenueBreakdown{
		TotalPaise:  totalPaise,
		BySource:    bySource,
		ByPlatform:  byPlatform,
		PeriodStart: start,
		PeriodEnd:   end,
		Growth:      calculateGrowth(totalPaise, prevTotal),
	}, nil
}

// ---------------------------------------------------------------------------
// Revenue Goals
// ---------------------------------------------------------------------------

// CreateGoal creates a new revenue goal. period_start and period_end are
// calculated automatically from the Period field.
func (s *RevenueAttributionService) CreateGoal(
	ctx context.Context,
	userID uuid.UUID,
	req *model.CreateGoalRequest,
) (*model.RevenueGoal, error) {

	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.TargetAmountPaise <= 0 {
		return nil, fmt.Errorf("target_amount_paise must be greater than zero")
	}

	validPeriods := map[string]bool{
		"weekly": true, "monthly": true, "quarterly": true, "yearly": true,
	}
	if !validPeriods[req.Period] {
		return nil, fmt.Errorf("invalid period: %s (must be weekly, monthly, quarterly, yearly)", req.Period)
	}

	periodStart, periodEnd := calculatePeriodDates(req.Period)

	goal := &model.RevenueGoal{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO revenue_goals (
			user_id, name, target_amount_paise, period,
			period_start, period_end, source_types
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, name, target_amount_paise, current_amount_paise,
			period, period_start, period_end, source_types, status,
			achieved_at, created_at, updated_at`,
		userID, req.Name, req.TargetAmountPaise, req.Period,
		periodStart, periodEnd, req.SourceTypes,
	).Scan(
		&goal.ID, &goal.UserID, &goal.Name,
		&goal.TargetAmountPaise, &goal.CurrentAmountPaise,
		&goal.Period, &goal.PeriodStart, &goal.PeriodEnd,
		&goal.SourceTypes, &goal.Status, &goal.AchievedAt,
		&goal.CreatedAt, &goal.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert revenue goal: %w", err)
	}

	if goal.TargetAmountPaise > 0 {
		goal.ProgressPercent = float64(goal.CurrentAmountPaise) / float64(goal.TargetAmountPaise) * 100
	}

	return goal, nil
}

// ListGoals returns all revenue goals for a user, ordered by creation date descending.
func (s *RevenueAttributionService) ListGoals(
	ctx context.Context,
	userID uuid.UUID,
) ([]model.RevenueGoal, error) {

	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, target_amount_paise, current_amount_paise,
			period, period_start, period_end, source_types, status,
			achieved_at, created_at, updated_at
		FROM revenue_goals
		WHERE user_id = $1
		ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query revenue goals: %w", err)
	}
	defer rows.Close()

	var goals []model.RevenueGoal
	for rows.Next() {
		var g model.RevenueGoal
		if err := rows.Scan(
			&g.ID, &g.UserID, &g.Name,
			&g.TargetAmountPaise, &g.CurrentAmountPaise,
			&g.Period, &g.PeriodStart, &g.PeriodEnd,
			&g.SourceTypes, &g.Status, &g.AchievedAt,
			&g.CreatedAt, &g.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan revenue goal: %w", err)
		}
		if g.TargetAmountPaise > 0 {
			g.ProgressPercent = float64(g.CurrentAmountPaise) / float64(g.TargetAmountPaise) * 100
		}
		goals = append(goals, g)
	}

	return goals, rows.Err()
}

// GetGoalProgress calculates detailed progress for a single goal, including
// whether the user is on track based on elapsed time vs progress.
func (s *RevenueAttributionService) GetGoalProgress(
	ctx context.Context,
	userID, goalID uuid.UUID,
) (*model.GoalProgress, error) {

	var goal model.RevenueGoal
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, name, target_amount_paise, period,
			period_start, period_end, source_types, status
		FROM revenue_goals
		WHERE id = $1 AND user_id = $2`,
		goalID, userID,
	).Scan(
		&goal.ID, &goal.UserID, &goal.Name,
		&goal.TargetAmountPaise, &goal.Period,
		&goal.PeriodStart, &goal.PeriodEnd,
		&goal.SourceTypes, &goal.Status,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("goal not found")
		}
		return nil, fmt.Errorf("query goal: %w", err)
	}

	// Calculate current amount from matching revenue_sources
	var currentPaise int64
	if len(goal.SourceTypes) == 0 {
		// All source types
		err = s.pool.QueryRow(ctx, `
			SELECT COALESCE(SUM(amount_paise), 0)
			FROM revenue_sources
			WHERE user_id = $1
			  AND period_start >= $2
			  AND period_end <= $3`,
			userID, goal.PeriodStart, goal.PeriodEnd,
		).Scan(&currentPaise)
	} else {
		// Filtered source types
		err = s.pool.QueryRow(ctx, `
			SELECT COALESCE(SUM(amount_paise), 0)
			FROM revenue_sources
			WHERE user_id = $1
			  AND period_start >= $2
			  AND period_end <= $3
			  AND source_type = ANY($4)`,
			userID, goal.PeriodStart, goal.PeriodEnd, goal.SourceTypes,
		).Scan(&currentPaise)
	}
	if err != nil {
		return nil, fmt.Errorf("calculate goal current amount: %w", err)
	}

	// Calculate progress percentage
	var progressPercent float64
	if goal.TargetAmountPaise > 0 {
		progressPercent = float64(currentPaise) / float64(goal.TargetAmountPaise) * 100
		progressPercent = math.Min(progressPercent, 100)
	}

	// Calculate days remaining
	now := time.Now().UTC()
	daysRemaining := int(goal.PeriodEnd.Sub(now).Hours() / 24)
	if daysRemaining < 0 {
		daysRemaining = 0
	}

	// Determine if on track:
	// Expected progress based on elapsed time vs total time
	totalDays := goal.PeriodEnd.Sub(goal.PeriodStart).Hours() / 24
	elapsedDays := now.Sub(goal.PeriodStart).Hours() / 24
	if elapsedDays < 0 {
		elapsedDays = 0
	}

	var onTrack bool
	if totalDays > 0 {
		expectedProgress := (elapsedDays / totalDays) * 100
		onTrack = progressPercent >= expectedProgress
	}

	return &model.GoalProgress{
		GoalID:          goal.ID,
		Name:            goal.Name,
		TargetPaise:     goal.TargetAmountPaise,
		CurrentPaise:    currentPaise,
		ProgressPercent: progressPercent,
		DaysRemaining:   daysRemaining,
		OnTrack:         onTrack,
	}, nil
}

// UpdateGoalProgress recalculates current_amount_paise for all active goals.
// Marks goals as "achieved" when current >= target, and "expired" when
// period_end < now. Called by the GoalUpdateWorker.
func (s *RevenueAttributionService) UpdateGoalProgress(ctx context.Context) error {
	// Fetch all active goals
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, target_amount_paise, period_start, period_end, source_types
		FROM revenue_goals
		WHERE status = 'active'`)
	if err != nil {
		return fmt.Errorf("query active goals: %w", err)
	}
	defer rows.Close()

	type activeGoal struct {
		ID          uuid.UUID
		UserID      uuid.UUID
		TargetPaise int64
		PeriodStart time.Time
		PeriodEnd   time.Time
		SourceTypes []string
	}

	var goals []activeGoal
	for rows.Next() {
		var g activeGoal
		if err := rows.Scan(
			&g.ID, &g.UserID, &g.TargetPaise,
			&g.PeriodStart, &g.PeriodEnd, &g.SourceTypes,
		); err != nil {
			return fmt.Errorf("scan active goal: %w", err)
		}
		goals = append(goals, g)
	}
	if rows.Err() != nil {
		return rows.Err()
	}

	now := time.Now().UTC()

	for _, g := range goals {
		// Calculate current revenue for this goal
		var currentPaise int64
		if len(g.SourceTypes) == 0 {
			err = s.pool.QueryRow(ctx, `
				SELECT COALESCE(SUM(amount_paise), 0)
				FROM revenue_sources
				WHERE user_id = $1
				  AND period_start >= $2
				  AND period_end <= $3`,
				g.UserID, g.PeriodStart, g.PeriodEnd,
			).Scan(&currentPaise)
		} else {
			err = s.pool.QueryRow(ctx, `
				SELECT COALESCE(SUM(amount_paise), 0)
				FROM revenue_sources
				WHERE user_id = $1
				  AND period_start >= $2
				  AND period_end <= $3
				  AND source_type = ANY($4)`,
				g.UserID, g.PeriodStart, g.PeriodEnd, g.SourceTypes,
			).Scan(&currentPaise)
		}
		if err != nil {
			return fmt.Errorf("calculate goal %s current amount: %w", g.ID, err)
		}

		// Determine new status
		newStatus := "active"
		var achievedAt *time.Time

		if currentPaise >= g.TargetPaise {
			newStatus = "achieved"
			t := now
			achievedAt = &t
		} else if g.PeriodEnd.Before(now) {
			newStatus = "expired"
		}

		// Update the goal
		_, err = s.pool.Exec(ctx, `
			UPDATE revenue_goals
			SET current_amount_paise = $2,
				status = $3,
				achieved_at = COALESCE($4, achieved_at),
				updated_at = NOW()
			WHERE id = $1`,
			g.ID, currentPaise, newStatus, achievedAt,
		)
		if err != nil {
			return fmt.Errorf("update goal %s: %w", g.ID, err)
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Affiliate Links
// ---------------------------------------------------------------------------

// CreateAffiliateLink generates a tracking code and creates a new affiliate link.
func (s *RevenueAttributionService) CreateAffiliateLink(
	ctx context.Context,
	userID uuid.UUID,
	req *model.CreateAffiliateLinkRequest,
) (*model.AffiliateLink, error) {

	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.OriginalURL == "" {
		return nil, fmt.Errorf("original_url is required")
	}

	trackingCode := generateTrackingCode()

	link := &model.AffiliateLink{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO affiliate_links (
			user_id, name, original_url, tracking_code
		) VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, name, original_url, tracking_code, short_url,
			click_count, conversion_count, revenue_paise, is_active,
			created_at, updated_at`,
		userID, req.Name, req.OriginalURL, trackingCode,
	).Scan(
		&link.ID, &link.UserID, &link.Name, &link.OriginalURL,
		&link.TrackingCode, &link.ShortURL, &link.ClickCount,
		&link.ConversionCount, &link.RevenuePaise, &link.IsActive,
		&link.CreatedAt, &link.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert affiliate link: %w", err)
	}

	return link, nil
}

// ListAffiliateLinks returns all affiliate links for a user.
func (s *RevenueAttributionService) ListAffiliateLinks(
	ctx context.Context,
	userID uuid.UUID,
) ([]model.AffiliateLink, error) {

	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, original_url, tracking_code, short_url,
			click_count, conversion_count, revenue_paise, is_active,
			created_at, updated_at
		FROM affiliate_links
		WHERE user_id = $1
		ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query affiliate links: %w", err)
	}
	defer rows.Close()

	var links []model.AffiliateLink
	for rows.Next() {
		var link model.AffiliateLink
		if err := rows.Scan(
			&link.ID, &link.UserID, &link.Name, &link.OriginalURL,
			&link.TrackingCode, &link.ShortURL, &link.ClickCount,
			&link.ConversionCount, &link.RevenuePaise, &link.IsActive,
			&link.CreatedAt, &link.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan affiliate link: %w", err)
		}
		links = append(links, link)
	}

	return links, rows.Err()
}

// TrackClick atomically increments the click count for an affiliate link
// and returns the original URL for redirect. Returns an error if the
// tracking code is not found or the link is inactive.
func (s *RevenueAttributionService) TrackClick(
	ctx context.Context,
	trackingCode string,
) (string, error) {

	var originalURL string
	err := s.pool.QueryRow(ctx, `
		UPDATE affiliate_links
		SET click_count = click_count + 1,
			updated_at = NOW()
		WHERE tracking_code = $1 AND is_active = TRUE
		RETURNING original_url`,
		trackingCode,
	).Scan(&originalURL)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("affiliate link not found or inactive")
		}
		return "", fmt.Errorf("track click: %w", err)
	}

	return originalURL, nil
}

// RecordConversion atomically increments the conversion count and adds revenue
// for an affiliate link identified by tracking code.
func (s *RevenueAttributionService) RecordConversion(
	ctx context.Context,
	trackingCode string,
	revenuePaise int64,
) error {

	if revenuePaise < 0 {
		return fmt.Errorf("revenue_paise must be non-negative")
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE affiliate_links
		SET conversion_count = conversion_count + 1,
			revenue_paise = revenue_paise + $2,
			updated_at = NOW()
		WHERE tracking_code = $1 AND is_active = TRUE`,
		trackingCode, revenuePaise,
	)
	if err != nil {
		return fmt.Errorf("record conversion: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("affiliate link not found or inactive")
	}

	return nil
}
