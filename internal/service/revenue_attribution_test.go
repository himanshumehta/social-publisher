package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ---------------------------------------------------------------------------
// Pure function tests (no DB)
// ---------------------------------------------------------------------------

func TestCalculatePeriodDates_Monthly(t *testing.T) {
	start, end := calculatePeriodDates("monthly")

	now := time.Now().UTC()
	expectedStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	expectedEnd := expectedStart.AddDate(0, 1, 0)

	if !start.Equal(expectedStart) {
		t.Errorf("monthly start: expected %v, got %v", expectedStart, start)
	}
	if !end.Equal(expectedEnd) {
		t.Errorf("monthly end: expected %v, got %v", expectedEnd, end)
	}
}

func TestCalculatePeriodDates_Weekly(t *testing.T) {
	start, end := calculatePeriodDates("weekly")

	// Verify start is a Monday
	if start.Weekday() != time.Monday {
		t.Errorf("weekly start should be Monday, got %s", start.Weekday())
	}

	// Verify the period is exactly 7 days
	diff := end.Sub(start)
	if diff != 7*24*time.Hour {
		t.Errorf("weekly period should be 7 days, got %v", diff)
	}
}

func TestCalculatePeriodDates_Quarterly(t *testing.T) {
	start, end := calculatePeriodDates("quarterly")

	// Start month should be first month of a quarter (Jan, Apr, Jul, Oct)
	validStarts := map[time.Month]bool{
		time.January: true, time.April: true,
		time.July: true, time.October: true,
	}
	if !validStarts[start.Month()] {
		t.Errorf("quarterly start month should be Jan/Apr/Jul/Oct, got %s", start.Month())
	}

	// Verify the period is 3 months
	expectedEnd := start.AddDate(0, 3, 0)
	if !end.Equal(expectedEnd) {
		t.Errorf("quarterly end: expected %v, got %v", expectedEnd, end)
	}
}

func TestCalculatePeriodDates_Yearly(t *testing.T) {
	start, end := calculatePeriodDates("yearly")

	now := time.Now().UTC()
	expectedStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	expectedEnd := expectedStart.AddDate(1, 0, 0)

	if !start.Equal(expectedStart) {
		t.Errorf("yearly start: expected %v, got %v", expectedStart, start)
	}
	if !end.Equal(expectedEnd) {
		t.Errorf("yearly end: expected %v, got %v", expectedEnd, end)
	}
}

func TestCalculateGrowth_Positive(t *testing.T) {
	growth := calculateGrowth(200, 100)
	expected := 100.0
	if growth != expected {
		t.Errorf("expected %.2f, got %.2f", expected, growth)
	}
}

func TestCalculateGrowth_Negative(t *testing.T) {
	growth := calculateGrowth(50, 100)
	expected := -50.0
	if growth != expected {
		t.Errorf("expected %.2f, got %.2f", expected, growth)
	}
}

func TestCalculateGrowth_ZeroPrevious(t *testing.T) {
	// Non-zero current, zero previous => 100
	growth := calculateGrowth(500, 0)
	if growth != 100 {
		t.Errorf("expected 100 for zero previous, got %.2f", growth)
	}

	// Both zero => 0
	growth = calculateGrowth(0, 0)
	if growth != 0 {
		t.Errorf("expected 0 for both zero, got %.2f", growth)
	}
}

func TestGenerateTrackingCode_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		code := generateTrackingCode()
		if len(code) != 8 {
			t.Fatalf("expected tracking code length 8, got %d: %q", len(code), code)
		}
		if seen[code] {
			t.Fatalf("duplicate tracking code after %d iterations: %s", i, code)
		}
		seen[code] = true
	}
}

// ---------------------------------------------------------------------------
// Database-backed tests
// ---------------------------------------------------------------------------

// createRevenueTestUser creates a test user and returns the user ID.
func createRevenueTestUser(t *testing.T, pool interface {
	Exec(ctx context.Context, sql string, arguments ...any) (interface{ RowsAffected() int64 }, error)
}, ctx context.Context) uuid.UUID {
	t.Helper()
	// Placeholder -- actual helper below uses pgxpool.Pool directly
	return uuid.Nil
}

func TestGetBreakdown_AggregatesByType(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewRevenueAttributionService(pool)
	ctx := context.Background()

	userID := uuid.New()
	email := fmt.Sprintf("rev-test-%s@example.com", uuid.New().String()[:8])
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Revenue Test User", email, "creator",
	)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	defer func() {
		pool.Exec(ctx, `DELETE FROM revenue_sources WHERE user_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	}()

	// Add sources of different types within the same period
	sources := []model.CreateRevenueSourceRequest{
		{Name: "YT Ads Jan", SourceType: model.SourceTypePlatformAd, AmountPaise: 500000, PeriodStart: "2025-01-01", PeriodEnd: "2025-01-31"},
		{Name: "Brand Deal", SourceType: model.SourceTypeSponsorship, AmountPaise: 1000000, PeriodStart: "2025-01-01", PeriodEnd: "2025-01-31"},
		{Name: "Merch Sales", SourceType: model.SourceTypeMerchandise, AmountPaise: 200000, PeriodStart: "2025-01-01", PeriodEnd: "2025-01-31"},
	}

	for _, req := range sources {
		r := req
		_, err := svc.AddSource(ctx, userID, &r)
		if err != nil {
			t.Fatalf("add source %s: %v", req.Name, err)
		}
	}

	start, _ := time.Parse("2006-01-02", "2025-01-01")
	end, _ := time.Parse("2006-01-02", "2025-01-31")

	breakdown, err := svc.GetBreakdown(ctx, userID, start, end)
	if err != nil {
		t.Fatalf("get breakdown: %v", err)
	}

	expectedTotal := int64(500000 + 1000000 + 200000)
	if breakdown.TotalPaise != expectedTotal {
		t.Errorf("total: expected %d, got %d", expectedTotal, breakdown.TotalPaise)
	}

	if breakdown.BySource["platform_ad"] != 500000 {
		t.Errorf("platform_ad: expected 500000, got %d", breakdown.BySource["platform_ad"])
	}
	if breakdown.BySource["sponsorship"] != 1000000 {
		t.Errorf("sponsorship: expected 1000000, got %d", breakdown.BySource["sponsorship"])
	}
	if breakdown.BySource["merchandise"] != 200000 {
		t.Errorf("merchandise: expected 200000, got %d", breakdown.BySource["merchandise"])
	}
}

func TestGetBreakdown_AggregatesByPlatform(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewRevenueAttributionService(pool)
	ctx := context.Background()

	userID := uuid.New()
	email := fmt.Sprintf("rev-plat-%s@example.com", uuid.New().String()[:8])
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Platform Test User", email, "creator",
	)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	defer func() {
		pool.Exec(ctx, `DELETE FROM revenue_sources WHERE user_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	}()

	yt := "youtube"
	ig := "instagram"

	sources := []model.CreateRevenueSourceRequest{
		{Name: "YT Ads", SourceType: model.SourceTypePlatformAd, Platform: &yt, AmountPaise: 300000, PeriodStart: "2025-02-01", PeriodEnd: "2025-02-28"},
		{Name: "IG Sponsor", SourceType: model.SourceTypeSponsorship, Platform: &ig, AmountPaise: 700000, PeriodStart: "2025-02-01", PeriodEnd: "2025-02-28"},
		{Name: "YT Sponsor", SourceType: model.SourceTypeSponsorship, Platform: &yt, AmountPaise: 500000, PeriodStart: "2025-02-01", PeriodEnd: "2025-02-28"},
	}

	for _, req := range sources {
		r := req
		_, err := svc.AddSource(ctx, userID, &r)
		if err != nil {
			t.Fatalf("add source %s: %v", req.Name, err)
		}
	}

	start, _ := time.Parse("2006-01-02", "2025-02-01")
	end, _ := time.Parse("2006-01-02", "2025-02-28")

	breakdown, err := svc.GetBreakdown(ctx, userID, start, end)
	if err != nil {
		t.Fatalf("get breakdown: %v", err)
	}

	// YouTube: 300000 + 500000 = 800000
	if breakdown.ByPlatform["youtube"] != 800000 {
		t.Errorf("youtube: expected 800000, got %d", breakdown.ByPlatform["youtube"])
	}
	// Instagram: 700000
	if breakdown.ByPlatform["instagram"] != 700000 {
		t.Errorf("instagram: expected 700000, got %d", breakdown.ByPlatform["instagram"])
	}
}

func TestGetGoalProgress_OnTrack(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewRevenueAttributionService(pool)
	ctx := context.Background()

	userID := uuid.New()
	email := fmt.Sprintf("goal-on-%s@example.com", uuid.New().String()[:8])
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Goal OnTrack User", email, "creator",
	)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	defer func() {
		pool.Exec(ctx, `DELETE FROM revenue_sources WHERE user_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM revenue_goals WHERE user_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	}()

	// Create a monthly goal for 1,000,000 paise
	goal, err := svc.CreateGoal(ctx, userID, &model.CreateGoalRequest{
		Name:              "Monthly Target",
		TargetAmountPaise: 1000000,
		Period:            "monthly",
		SourceTypes:       []string{},
	})
	if err != nil {
		t.Fatalf("create goal: %v", err)
	}

	// Add revenue that puts us ahead of expected pace
	// For a monthly goal, if we are mid-month, we want > 50% to be "on track"
	_, err = svc.AddSource(ctx, userID, &model.CreateRevenueSourceRequest{
		Name:        "Big Sponsorship",
		SourceType:  model.SourceTypeSponsorship,
		AmountPaise: 900000,
		PeriodStart: goal.PeriodStart.Format("2006-01-02"),
		PeriodEnd:   goal.PeriodEnd.Format("2006-01-02"),
	})
	if err != nil {
		t.Fatalf("add source: %v", err)
	}

	progress, err := svc.GetGoalProgress(ctx, userID, goal.ID)
	if err != nil {
		t.Fatalf("get goal progress: %v", err)
	}

	if progress.CurrentPaise != 900000 {
		t.Errorf("current paise: expected 900000, got %d", progress.CurrentPaise)
	}
	if progress.ProgressPercent < 50 {
		t.Errorf("progress percent should be >= 50, got %.2f", progress.ProgressPercent)
	}
	// 90% progress should be on track for most of the month
	if !progress.OnTrack {
		t.Error("expected goal to be on track with 90% progress")
	}
}

func TestGetGoalProgress_BehindSchedule(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewRevenueAttributionService(pool)
	ctx := context.Background()

	userID := uuid.New()
	email := fmt.Sprintf("goal-behind-%s@example.com", uuid.New().String()[:8])
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Goal Behind User", email, "creator",
	)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	defer func() {
		pool.Exec(ctx, `DELETE FROM revenue_sources WHERE user_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM revenue_goals WHERE user_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	}()

	// Create a monthly goal for 10,000,000 paise (100,000 INR)
	goal, err := svc.CreateGoal(ctx, userID, &model.CreateGoalRequest{
		Name:              "Ambitious Target",
		TargetAmountPaise: 10000000,
		Period:            "monthly",
		SourceTypes:       []string{},
	})
	if err != nil {
		t.Fatalf("create goal: %v", err)
	}

	// Add only a tiny amount -- will be behind schedule
	_, err = svc.AddSource(ctx, userID, &model.CreateRevenueSourceRequest{
		Name:        "Small Donation",
		SourceType:  model.SourceTypeDonation,
		AmountPaise: 1000,
		PeriodStart: goal.PeriodStart.Format("2006-01-02"),
		PeriodEnd:   goal.PeriodEnd.Format("2006-01-02"),
	})
	if err != nil {
		t.Fatalf("add source: %v", err)
	}

	progress, err := svc.GetGoalProgress(ctx, userID, goal.ID)
	if err != nil {
		t.Fatalf("get goal progress: %v", err)
	}

	if progress.CurrentPaise != 1000 {
		t.Errorf("current paise: expected 1000, got %d", progress.CurrentPaise)
	}
	// 0.01% progress should be behind schedule unless it is the first day
	if progress.OnTrack && progress.DaysRemaining < 28 {
		t.Error("expected goal to be behind schedule with minimal progress")
	}
}

func TestUpdateGoalProgress_MarksAchieved(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewRevenueAttributionService(pool)
	ctx := context.Background()

	userID := uuid.New()
	email := fmt.Sprintf("goal-ach-%s@example.com", uuid.New().String()[:8])
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Goal Achieved User", email, "creator",
	)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	defer func() {
		pool.Exec(ctx, `DELETE FROM revenue_sources WHERE user_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM revenue_goals WHERE user_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	}()

	// Create a goal with a low target
	goal, err := svc.CreateGoal(ctx, userID, &model.CreateGoalRequest{
		Name:              "Easy Target",
		TargetAmountPaise: 10000,
		Period:            "monthly",
		SourceTypes:       []string{},
	})
	if err != nil {
		t.Fatalf("create goal: %v", err)
	}

	// Add revenue that exceeds the target
	_, err = svc.AddSource(ctx, userID, &model.CreateRevenueSourceRequest{
		Name:        "Big Payout",
		SourceType:  model.SourceTypePlatformAd,
		AmountPaise: 50000,
		PeriodStart: goal.PeriodStart.Format("2006-01-02"),
		PeriodEnd:   goal.PeriodEnd.Format("2006-01-02"),
	})
	if err != nil {
		t.Fatalf("add source: %v", err)
	}

	// Run the worker update
	err = svc.UpdateGoalProgress(ctx)
	if err != nil {
		t.Fatalf("update goal progress: %v", err)
	}

	// Verify goal is now achieved
	var status string
	var achievedAt *time.Time
	err = pool.QueryRow(ctx,
		`SELECT status, achieved_at FROM revenue_goals WHERE id = $1`,
		goal.ID,
	).Scan(&status, &achievedAt)
	if err != nil {
		t.Fatalf("query goal status: %v", err)
	}

	if status != "achieved" {
		t.Errorf("expected status 'achieved', got %q", status)
	}
	if achievedAt == nil {
		t.Error("expected achieved_at to be set")
	}
}

func TestUpdateGoalProgress_MarksExpired(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewRevenueAttributionService(pool)
	ctx := context.Background()

	userID := uuid.New()
	email := fmt.Sprintf("goal-exp-%s@example.com", uuid.New().String()[:8])
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Goal Expired User", email, "creator",
	)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	defer func() {
		pool.Exec(ctx, `DELETE FROM revenue_sources WHERE user_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM revenue_goals WHERE user_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	}()

	// Manually insert a goal with a past period_end so it will expire
	goalID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO revenue_goals (
			id, user_id, name, target_amount_paise, period,
			period_start, period_end, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		goalID, userID, "Past Goal", int64(1000000), "monthly",
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
		"active",
	)
	if err != nil {
		t.Fatalf("insert past goal: %v", err)
	}

	// Run the worker update
	err = svc.UpdateGoalProgress(ctx)
	if err != nil {
		t.Fatalf("update goal progress: %v", err)
	}

	// Verify goal is now expired
	var status string
	err = pool.QueryRow(ctx,
		`SELECT status FROM revenue_goals WHERE id = $1`,
		goalID,
	).Scan(&status)
	if err != nil {
		t.Fatalf("query goal status: %v", err)
	}

	if status != "expired" {
		t.Errorf("expected status 'expired', got %q", status)
	}
}
