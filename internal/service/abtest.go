package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type ABTestService struct {
	pool *pgxpool.Pool
}

func NewABTestService(pool *pgxpool.Pool) *ABTestService {
	return &ABTestService{pool: pool}
}

// ---------------------------------------------------------------------------
// CreateTest
// ---------------------------------------------------------------------------

func (s *ABTestService) CreateTest(ctx context.Context, userID uuid.UUID, req *model.CreateABTestRequest) (*model.ABTest, error) {
	if len(req.Variants) < 2 {
		return nil, fmt.Errorf("at least 2 variants required")
	}
	if len(req.Variants) > 4 {
		return nil, fmt.Errorf("maximum 4 variants allowed")
	}
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.DurationHours < 1 {
		req.DurationHours = 24
	}
	if req.Metric == "" {
		req.Metric = model.ABTestMetricEngagement
	}

	// Validate all variant post IDs belong to this user.
	for _, v := range req.Variants {
		var ownerID uuid.UUID
		err := s.pool.QueryRow(ctx,
			`SELECT author_id FROM posts WHERE id = $1`, v.PostID,
		).Scan(&ownerID)
		if err != nil {
			return nil, fmt.Errorf("post %s not found", v.PostID)
		}
		if ownerID != userID {
			return nil, fmt.Errorf("post %s does not belong to you", v.PostID)
		}
	}

	// Validate original post belongs to user.
	var origOwner uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT author_id FROM posts WHERE id = $1`, req.OriginalPostID,
	).Scan(&origOwner)
	if err != nil {
		return nil, fmt.Errorf("original post not found")
	}
	if origOwner != userID {
		return nil, fmt.Errorf("original post does not belong to you")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var test model.ABTest
	err = tx.QueryRow(ctx,
		`INSERT INTO ab_tests (user_id, original_post_id, name, metric, duration_hours)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, user_id, original_post_id, name, status, metric, duration_hours,
		           started_at, completed_at, winner_variant_id, created_at, updated_at`,
		userID, req.OriginalPostID, req.Name, string(req.Metric), req.DurationHours,
	).Scan(
		&test.ID, &test.UserID, &test.OriginalPostID, &test.Name,
		&test.Status, &test.Metric, &test.DurationHours,
		&test.StartedAt, &test.CompletedAt, &test.WinnerVariantID,
		&test.CreatedAt, &test.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert ab_test: %w", err)
	}

	// Insert variants. First variant (A) is control by default.
	for i, v := range req.Variants {
		isControl := (v.VariantLabel == "A") || (i == 0 && v.VariantLabel == "")
		label := v.VariantLabel
		if label == "" {
			label = string(rune('A' + i))
		}

		var variant model.ABTestVariant
		err = tx.QueryRow(ctx,
			`INSERT INTO ab_test_variants (ab_test_id, variant_label, post_id, is_control)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id, ab_test_id, variant_label, post_id, is_control,
			           views, likes, comments, shares, clicks, impressions,
			           engagement_rate, is_winner, created_at, updated_at`,
			test.ID, label, v.PostID, isControl,
		).Scan(
			&variant.ID, &variant.ABTestID, &variant.VariantLabel, &variant.PostID,
			&variant.IsControl,
			&variant.Views, &variant.Likes, &variant.Comments, &variant.Shares,
			&variant.Clicks, &variant.Impressions,
			&variant.EngagementRate, &variant.IsWinner,
			&variant.CreatedAt, &variant.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("insert variant %s: %w", label, err)
		}
		test.Variants = append(test.Variants, variant)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &test, nil
}

// ---------------------------------------------------------------------------
// GetTest
// ---------------------------------------------------------------------------

func (s *ABTestService) GetTest(ctx context.Context, userID, testID uuid.UUID) (*model.ABTest, error) {
	var test model.ABTest
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, original_post_id, name, status, metric, duration_hours,
		        started_at, completed_at, winner_variant_id, created_at, updated_at
		 FROM ab_tests
		 WHERE id = $1 AND user_id = $2`,
		testID, userID,
	).Scan(
		&test.ID, &test.UserID, &test.OriginalPostID, &test.Name,
		&test.Status, &test.Metric, &test.DurationHours,
		&test.StartedAt, &test.CompletedAt, &test.WinnerVariantID,
		&test.CreatedAt, &test.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("ab test not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query ab_test: %w", err)
	}

	variants, err := s.loadVariants(ctx, test.ID)
	if err != nil {
		return nil, err
	}
	test.Variants = variants

	return &test, nil
}

// ---------------------------------------------------------------------------
// ListTests
// ---------------------------------------------------------------------------

func (s *ABTestService) ListTests(ctx context.Context, userID uuid.UUID) ([]model.ABTest, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, original_post_id, name, status, metric, duration_hours,
		        started_at, completed_at, winner_variant_id, created_at, updated_at
		 FROM ab_tests
		 WHERE user_id = $1
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query ab_tests: %w", err)
	}
	defer rows.Close()

	var tests []model.ABTest
	for rows.Next() {
		var t model.ABTest
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.OriginalPostID, &t.Name,
			&t.Status, &t.Metric, &t.DurationHours,
			&t.StartedAt, &t.CompletedAt, &t.WinnerVariantID,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan ab_test: %w", err)
		}

		variants, err := s.loadVariants(ctx, t.ID)
		if err != nil {
			return nil, err
		}
		t.Variants = variants

		tests = append(tests, t)
	}

	return tests, nil
}

// ---------------------------------------------------------------------------
// StartTest
// ---------------------------------------------------------------------------

func (s *ABTestService) StartTest(ctx context.Context, userID, testID uuid.UUID) error {
	// Verify ownership and draft status.
	var status model.ABTestStatus
	var variantCount int

	err := s.pool.QueryRow(ctx,
		`SELECT t.status, (SELECT COUNT(*) FROM ab_test_variants WHERE ab_test_id = t.id)
		 FROM ab_tests t
		 WHERE t.id = $1 AND t.user_id = $2`,
		testID, userID,
	).Scan(&status, &variantCount)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("ab test not found")
	}
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}

	if status != model.ABTestStatusDraft {
		return fmt.Errorf("test must be in draft status to start (current: %s)", status)
	}
	if variantCount < 2 {
		return fmt.Errorf("test requires at least 2 variants (has %d)", variantCount)
	}

	_, err = s.pool.Exec(ctx,
		`UPDATE ab_tests
		 SET status = 'running', started_at = NOW(), updated_at = NOW()
		 WHERE id = $1`,
		testID,
	)
	if err != nil {
		return fmt.Errorf("start test: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// UpdateVariantMetrics
// ---------------------------------------------------------------------------

func (s *ABTestService) UpdateVariantMetrics(
	ctx context.Context,
	variantID uuid.UUID,
	views, likes, comments, shares, clicks, impressions int,
) error {
	// Calculate engagement rate: (likes + comments + shares) / views * 100
	engagementRate := 0.0
	if views > 0 {
		engagementRate = float64(likes+comments+shares) / float64(views) * 100
	}

	_, err := s.pool.Exec(ctx,
		`UPDATE ab_test_variants
		 SET views = $2, likes = $3, comments = $4, shares = $5,
		     clicks = $6, impressions = $7, engagement_rate = $8, updated_at = NOW()
		 WHERE id = $1`,
		variantID, views, likes, comments, shares, clicks, impressions, engagementRate,
	)
	if err != nil {
		return fmt.Errorf("update variant metrics: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// EvaluateTest
// ---------------------------------------------------------------------------

func (s *ABTestService) EvaluateTest(ctx context.Context, testID uuid.UUID) (*model.ABTestResult, error) {
	// Load test.
	var metric model.ABTestMetric
	err := s.pool.QueryRow(ctx,
		`SELECT metric FROM ab_tests WHERE id = $1`, testID,
	).Scan(&metric)
	if err != nil {
		return nil, fmt.Errorf("load test metric: %w", err)
	}

	variants, err := s.loadVariants(ctx, testID)
	if err != nil {
		return nil, err
	}
	if len(variants) < 2 {
		return nil, fmt.Errorf("test has fewer than 2 variants")
	}

	// Find the control variant.
	var control *model.ABTestVariant
	for i := range variants {
		if variants[i].IsControl {
			control = &variants[i]
			break
		}
	}
	if control == nil {
		// Fallback: variant A is control.
		for i := range variants {
			if variants[i].VariantLabel == "A" {
				control = &variants[i]
				break
			}
		}
	}
	if control == nil {
		control = &variants[0]
	}

	// Determine winner by the chosen metric.
	var winner *model.ABTestVariant
	var bestScore float64 = -1

	for i := range variants {
		score := s.metricScore(&variants[i], metric)
		if score > bestScore {
			bestScore = score
			winner = &variants[i]
		}
	}

	if winner == nil {
		return nil, fmt.Errorf("could not determine winner")
	}

	controlScore := s.metricScore(control, metric)
	winnerScore := s.metricScore(winner, metric)

	improvement := calculateImprovement(controlScore, winnerScore)
	confidence := calculateConfidence(
		control.Views, winner.Views,
		control.EngagementRate/100, winner.EngagementRate/100,
	)

	return &model.ABTestResult{
		TestID:          testID,
		WinnerVariantID: winner.ID,
		WinnerLabel:     winner.VariantLabel,
		Improvement:     math.Round(improvement*100) / 100,
		Confidence:      math.Round(confidence*100) / 100,
	}, nil
}

// ---------------------------------------------------------------------------
// SelectWinner (manual)
// ---------------------------------------------------------------------------

func (s *ABTestService) SelectWinner(ctx context.Context, userID, testID, variantID uuid.UUID) error {
	// Verify ownership.
	var ownerID uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT user_id FROM ab_tests WHERE id = $1`, testID,
	).Scan(&ownerID)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("ab test not found")
	}
	if err != nil {
		return fmt.Errorf("query test: %w", err)
	}
	if ownerID != userID {
		return fmt.Errorf("ab test not found")
	}

	// Verify variant belongs to this test.
	var exists bool
	err = s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM ab_test_variants WHERE id = $1 AND ab_test_id = $2)`,
		variantID, testID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check variant: %w", err)
	}
	if !exists {
		return fmt.Errorf("variant does not belong to this test")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Clear previous winner.
	_, err = tx.Exec(ctx,
		`UPDATE ab_test_variants SET is_winner = FALSE, updated_at = NOW()
		 WHERE ab_test_id = $1`,
		testID,
	)
	if err != nil {
		return fmt.Errorf("clear winners: %w", err)
	}

	// Set new winner.
	_, err = tx.Exec(ctx,
		`UPDATE ab_test_variants SET is_winner = TRUE, updated_at = NOW()
		 WHERE id = $1`,
		variantID,
	)
	if err != nil {
		return fmt.Errorf("set winner: %w", err)
	}

	// Complete the test.
	_, err = tx.Exec(ctx,
		`UPDATE ab_tests
		 SET status = 'completed', winner_variant_id = $2, completed_at = NOW(), updated_at = NOW()
		 WHERE id = $1`,
		testID, variantID,
	)
	if err != nil {
		return fmt.Errorf("complete test: %w", err)
	}

	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------------
// AutoEvaluate (used by worker)
// ---------------------------------------------------------------------------

func (s *ABTestService) AutoEvaluate(ctx context.Context) error {
	// Find running tests whose duration has expired.
	rows, err := s.pool.Query(ctx,
		`SELECT id FROM ab_tests
		 WHERE status = 'running'
		   AND started_at + (duration_hours || ' hours')::INTERVAL <= NOW()`,
	)
	if err != nil {
		return fmt.Errorf("query expired tests: %w", err)
	}
	defer rows.Close()

	var testIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan test id: %w", err)
		}
		testIDs = append(testIDs, id)
	}

	for _, testID := range testIDs {
		// Mark as evaluating.
		_, err := s.pool.Exec(ctx,
			`UPDATE ab_tests SET status = 'evaluating', updated_at = NOW() WHERE id = $1`,
			testID,
		)
		if err != nil {
			return fmt.Errorf("mark evaluating %s: %w", testID, err)
		}

		result, err := s.EvaluateTest(ctx, testID)
		if err != nil {
			// Mark back to running so it can be retried.
			s.pool.Exec(ctx,
				`UPDATE ab_tests SET status = 'running', updated_at = NOW() WHERE id = $1`,
				testID,
			)
			return fmt.Errorf("evaluate %s: %w", testID, err)
		}

		// Set the winner.
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}

		_, err = tx.Exec(ctx,
			`UPDATE ab_test_variants SET is_winner = FALSE, updated_at = NOW()
			 WHERE ab_test_id = $1`,
			testID,
		)
		if err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("clear winners: %w", err)
		}

		_, err = tx.Exec(ctx,
			`UPDATE ab_test_variants SET is_winner = TRUE, updated_at = NOW()
			 WHERE id = $1`,
			result.WinnerVariantID,
		)
		if err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("set winner: %w", err)
		}

		_, err = tx.Exec(ctx,
			`UPDATE ab_tests
			 SET status = 'completed', winner_variant_id = $2, completed_at = NOW(), updated_at = NOW()
			 WHERE id = $1`,
			testID, result.WinnerVariantID,
		)
		if err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("complete test: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit: %w", err)
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// CancelTest
// ---------------------------------------------------------------------------

func (s *ABTestService) CancelTest(ctx context.Context, userID, testID uuid.UUID) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE ab_tests
		 SET status = 'cancelled', updated_at = NOW()
		 WHERE id = $1 AND user_id = $2 AND status IN ('draft', 'running')`,
		testID, userID,
	)
	if err != nil {
		return fmt.Errorf("cancel test: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("ab test not found or cannot be cancelled")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (s *ABTestService) loadVariants(ctx context.Context, testID uuid.UUID) ([]model.ABTestVariant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, ab_test_id, variant_label, post_id, is_control,
		        views, likes, comments, shares, clicks, impressions,
		        engagement_rate, is_winner, created_at, updated_at
		 FROM ab_test_variants
		 WHERE ab_test_id = $1
		 ORDER BY variant_label ASC`,
		testID,
	)
	if err != nil {
		return nil, fmt.Errorf("query variants: %w", err)
	}
	defer rows.Close()

	var variants []model.ABTestVariant
	for rows.Next() {
		var v model.ABTestVariant
		if err := rows.Scan(
			&v.ID, &v.ABTestID, &v.VariantLabel, &v.PostID, &v.IsControl,
			&v.Views, &v.Likes, &v.Comments, &v.Shares, &v.Clicks, &v.Impressions,
			&v.EngagementRate, &v.IsWinner,
			&v.CreatedAt, &v.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan variant: %w", err)
		}
		variants = append(variants, v)
	}

	return variants, nil
}

// metricScore returns the relevant score for a variant based on the chosen metric.
func (s *ABTestService) metricScore(v *model.ABTestVariant, metric model.ABTestMetric) float64 {
	switch metric {
	case model.ABTestMetricEngagement:
		return v.EngagementRate
	case model.ABTestMetricClicks:
		return float64(v.Clicks)
	case model.ABTestMetricImpressions:
		return float64(v.Impressions)
	case model.ABTestMetricReach:
		return float64(v.Views)
	default:
		return v.EngagementRate
	}
}

// calculateImprovement returns the % improvement of variant over control.
// Returns 0 if control is zero (avoids division by zero).
func calculateImprovement(control, variant float64) float64 {
	if control == 0 {
		if variant == 0 {
			return 0
		}
		return 100 // variant is infinitely better; cap at 100%
	}
	return (variant - control) / control * 100
}

// calculateConfidence uses a two-proportion z-test approximation to estimate
// statistical significance. Returns a value between 0 and 100 (percentage).
//
// Parameters:
//   - controlN, variantN: number of views (sample sizes)
//   - controlRate, variantRate: engagement rates as decimals (e.g. 0.05 for 5%)
func calculateConfidence(controlN, variantN int, controlRate, variantRate float64) float64 {
	if controlN == 0 || variantN == 0 {
		return 0
	}

	n1 := float64(controlN)
	n2 := float64(variantN)

	// Pooled proportion.
	pooled := (controlRate*n1 + variantRate*n2) / (n1 + n2)
	if pooled == 0 || pooled == 1 {
		return 0
	}

	// Standard error.
	se := math.Sqrt(pooled * (1 - pooled) * (1/n1 + 1/n2))
	if se == 0 {
		return 0
	}

	// Z-score.
	z := math.Abs(variantRate-controlRate) / se

	// Convert z-score to approximate confidence using the error function.
	// P(Z < z) approx using erf.
	confidence := math.Erf(z/math.Sqrt(2)) * 100

	if confidence > 99.99 {
		confidence = 99.99
	}

	return confidence
}

// Ensure time import is used (compile guard).
var _ = time.Now
