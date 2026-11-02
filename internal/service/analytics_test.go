package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ---------------------------------------------------------------------------
// Pure function tests (no DB)
// ---------------------------------------------------------------------------

func TestCalculateEngagementRate_Normal(t *testing.T) {
	rate := calculateEngagementRate(50, 20, 10, 1000)
	// (50+20+10)/1000*100 = 8.0
	expected := 8.0
	if rate != expected {
		t.Errorf("expected %.4f, got %.4f", expected, rate)
	}
}

func TestCalculateEngagementRate_ZeroViews(t *testing.T) {
	rate := calculateEngagementRate(50, 20, 10, 0)
	if rate != 0 {
		t.Errorf("expected 0 for zero views, got %.4f", rate)
	}
}

// ---------------------------------------------------------------------------
// Database-backed tests
// ---------------------------------------------------------------------------

func TestGetOverview_WithData(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewAnalyticsService(pool)
	ctx := context.Background()

	userID := findUserByRole(t, pool, "creator")
	channelID := createTestChannel(t, pool)

	today := time.Now().UTC().Truncate(24 * time.Hour)
	dr := model.DateRange{Start: today.AddDate(0, 0, -7), End: today.AddDate(0, 0, 1)}

	// Insert 3 days of snapshots.
	for i := 0; i < 3; i++ {
		snap := &model.AnalyticsSnapshot{
			UserID:       userID,
			ChannelID:    &channelID,
			Platform:     "youtube",
			SnapshotDate: today.AddDate(0, 0, -i),
			Followers:    1000 + i*100,
			Views:        5000 + i*500,
			Likes:        200 + i*20,
			Comments:     50 + i*5,
			Shares:       30 + i*3,
			RevenuePaise: 100000,
		}
		if err := svc.SaveSnapshot(ctx, snap); err != nil {
			t.Fatalf("save snapshot: %v", err)
		}
	}

	t.Cleanup(func() {
		pool.Exec(context.Background(),
			`DELETE FROM analytics_snapshots WHERE user_id = $1`, userID)
	})

	overview, err := svc.GetOverview(ctx, userID, dr)
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}

	if overview.TotalViews == 0 {
		t.Error("expected non-zero total views")
	}
	if len(overview.PlatformBreakdown) == 0 {
		t.Error("expected at least one platform in breakdown")
	}
	if overview.PlatformBreakdown[0].Platform != "youtube" {
		t.Errorf("expected platform youtube, got %s", overview.PlatformBreakdown[0].Platform)
	}
}

func TestGetOverview_EmptyRange(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewAnalyticsService(pool)
	ctx := context.Background()

	userID := findUserByRole(t, pool, "creator")

	// Far-future range with no data.
	dr := model.DateRange{
		Start: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2099, 2, 1, 0, 0, 0, 0, time.UTC),
	}

	overview, err := svc.GetOverview(ctx, userID, dr)
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}

	if overview.TotalViews != 0 {
		t.Errorf("expected 0 views for empty range, got %d", overview.TotalViews)
	}
	if overview.TotalFollowers != 0 {
		t.Errorf("expected 0 followers for empty range, got %d", overview.TotalFollowers)
	}
}

func TestGetPlatformStats_DateFiltering(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewAnalyticsService(pool)
	ctx := context.Background()

	userID := findUserByRole(t, pool, "creator")
	channelID := createTestChannel(t, pool)

	base := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	// Insert 10 days.
	for i := 0; i < 10; i++ {
		snap := &model.AnalyticsSnapshot{
			UserID:       userID,
			ChannelID:    &channelID,
			Platform:     "instagram",
			SnapshotDate: base.AddDate(0, 0, i),
			Followers:    500,
			Views:        1000,
			Likes:        100,
			Comments:     20,
			Shares:       5,
		}
		if err := svc.SaveSnapshot(ctx, snap); err != nil {
			t.Fatalf("save snapshot: %v", err)
		}
	}

	t.Cleanup(func() {
		pool.Exec(context.Background(),
			`DELETE FROM analytics_snapshots WHERE user_id = $1`, userID)
		cleanupChannel(t, pool, channelID)
	})

	// Query only 5 days.
	dr := model.DateRange{Start: base, End: base.AddDate(0, 0, 5)}
	stats, err := svc.GetPlatformStats(ctx, userID, "instagram", dr)
	if err != nil {
		t.Fatalf("get platform stats: %v", err)
	}

	if len(stats) != 5 {
		t.Errorf("expected 5 snapshots, got %d", len(stats))
	}

	// Verify ordering.
	for i := 1; i < len(stats); i++ {
		if stats[i].SnapshotDate.Before(stats[i-1].SnapshotDate) {
			t.Error("snapshots not in ascending date order")
		}
	}
}

func TestGetComparison_GrowthCalculation(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewAnalyticsService(pool)
	ctx := context.Background()

	userID := findUserByRole(t, pool, "creator")
	channelID := createTestChannel(t, pool)

	base := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	// Previous period: lower numbers.
	prevSnap := &model.AnalyticsSnapshot{
		UserID:       userID,
		ChannelID:    &channelID,
		Platform:     "tiktok",
		SnapshotDate: base,
		Followers:    1000,
		Views:        5000,
		Likes:        200,
		Comments:     50,
		Shares:       30,
	}
	if err := svc.SaveSnapshot(ctx, prevSnap); err != nil {
		t.Fatalf("save prev snapshot: %v", err)
	}

	// Current period: higher numbers.
	currSnap := &model.AnalyticsSnapshot{
		UserID:       userID,
		ChannelID:    &channelID,
		Platform:     "tiktok",
		SnapshotDate: base.AddDate(0, 0, 7),
		Followers:    2000,
		Views:        10000,
		Likes:        500,
		Comments:     100,
		Shares:       80,
	}
	if err := svc.SaveSnapshot(ctx, currSnap); err != nil {
		t.Fatalf("save curr snapshot: %v", err)
	}

	t.Cleanup(func() {
		pool.Exec(context.Background(),
			`DELETE FROM analytics_snapshots WHERE user_id = $1`, userID)
		cleanupChannel(t, pool, channelID)
	})

	current := model.DateRange{Start: base.AddDate(0, 0, 7), End: base.AddDate(0, 0, 14)}
	previous := model.DateRange{Start: base, End: base.AddDate(0, 0, 7)}

	comp, err := svc.GetComparison(ctx, userID, current, previous)
	if err != nil {
		t.Fatalf("get comparison: %v", err)
	}

	if comp.CurrentPeriod.Views <= comp.PreviousPeriod.Views {
		t.Error("expected current views > previous views")
	}
	if comp.CurrentPeriod.Engagements <= comp.PreviousPeriod.Engagements {
		t.Error("expected current engagements > previous engagements")
	}
}

func TestGetComparison_PreviousPeriodEmpty(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewAnalyticsService(pool)
	ctx := context.Background()

	userID := findUserByRole(t, pool, "creator")
	channelID := createTestChannel(t, pool)

	base := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)

	// Only current period has data.
	snap := &model.AnalyticsSnapshot{
		UserID:       userID,
		ChannelID:    &channelID,
		Platform:     "youtube",
		SnapshotDate: base,
		Followers:    500,
		Views:        2000,
		Likes:        100,
		Comments:     30,
		Shares:       10,
	}
	if err := svc.SaveSnapshot(ctx, snap); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	t.Cleanup(func() {
		pool.Exec(context.Background(),
			`DELETE FROM analytics_snapshots WHERE user_id = $1`, userID)
		cleanupChannel(t, pool, channelID)
	})

	current := model.DateRange{Start: base, End: base.AddDate(0, 0, 7)}
	previous := model.DateRange{Start: base.AddDate(0, 0, -7), End: base}

	comp, err := svc.GetComparison(ctx, userID, current, previous)
	if err != nil {
		t.Fatalf("get comparison: %v", err)
	}

	// Previous period should be zeroes.
	if comp.PreviousPeriod.Views != 0 {
		t.Errorf("expected 0 previous views, got %d", comp.PreviousPeriod.Views)
	}
	// Growth should show 100% (new data from nothing).
	growth := growthPct(float64(comp.CurrentPeriod.Views), float64(comp.PreviousPeriod.Views))
	if growth != 100 {
		t.Errorf("expected 100%% growth from zero, got %.2f%%", growth)
	}
}

func TestSaveSnapshot_Insert(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewAnalyticsService(pool)
	ctx := context.Background()

	userID := findUserByRole(t, pool, "creator")
	channelID := createTestChannel(t, pool)

	snap := &model.AnalyticsSnapshot{
		UserID:       userID,
		ChannelID:    &channelID,
		Platform:     "youtube",
		SnapshotDate: time.Now().UTC().Truncate(24 * time.Hour),
		Followers:    1234,
		Views:        5678,
		Likes:        200,
		Comments:     50,
		Shares:       25,
		Impressions:  10000,
		Reach:        8000,
		Clicks:       300,
		RevenuePaise: 500000,
		Metadata:     map[string]interface{}{"source": "test"},
	}

	err := svc.SaveSnapshot(ctx, snap)
	if err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}

	t.Cleanup(func() {
		pool.Exec(context.Background(),
			`DELETE FROM analytics_snapshots WHERE user_id = $1`, userID)
		cleanupChannel(t, pool, channelID)
	})

	// Verify it exists.
	dr := model.DateRange{
		Start: snap.SnapshotDate,
		End:   snap.SnapshotDate.AddDate(0, 0, 1),
	}
	stats, err := svc.GetPlatformStats(ctx, userID, "youtube", dr)
	if err != nil {
		t.Fatalf("get platform stats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(stats))
	}
	if stats[0].Followers != 1234 {
		t.Errorf("expected 1234 followers, got %d", stats[0].Followers)
	}
}

func TestSaveSnapshot_Upsert(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewAnalyticsService(pool)
	ctx := context.Background()

	userID := findUserByRole(t, pool, "creator")
	channelID := createTestChannel(t, pool)

	snapDate := time.Date(2025, 8, 15, 0, 0, 0, 0, time.UTC)

	// First insert.
	snap1 := &model.AnalyticsSnapshot{
		UserID:       userID,
		ChannelID:    &channelID,
		Platform:     "youtube",
		SnapshotDate: snapDate,
		Followers:    1000,
		Views:        5000,
		Likes:        200,
		Comments:     50,
		Shares:       30,
	}
	if err := svc.SaveSnapshot(ctx, snap1); err != nil {
		t.Fatalf("first save: %v", err)
	}

	// Upsert with updated values (same channel + platform + date).
	snap2 := &model.AnalyticsSnapshot{
		UserID:       userID,
		ChannelID:    &channelID,
		Platform:     "youtube",
		SnapshotDate: snapDate,
		Followers:    1500,
		Views:        8000,
		Likes:        400,
		Comments:     100,
		Shares:       60,
	}
	if err := svc.SaveSnapshot(ctx, snap2); err != nil {
		t.Fatalf("upsert save: %v", err)
	}

	t.Cleanup(func() {
		pool.Exec(context.Background(),
			`DELETE FROM analytics_snapshots WHERE user_id = $1`, userID)
		cleanupChannel(t, pool, channelID)
	})

	// Should still be one row, with updated values.
	dr := model.DateRange{Start: snapDate, End: snapDate.AddDate(0, 0, 1)}
	stats, err := svc.GetPlatformStats(ctx, userID, "youtube", dr)
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 snapshot after upsert, got %d", len(stats))
	}
	if stats[0].Followers != 1500 {
		t.Errorf("expected updated followers 1500, got %d", stats[0].Followers)
	}
	if stats[0].Views != 8000 {
		t.Errorf("expected updated views 8000, got %d", stats[0].Views)
	}
}

func TestAggregateDaily(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewAnalyticsService(pool)
	ctx := context.Background()

	userID := findUserByRole(t, pool, "creator")
	channelID := createTestChannel(t, pool)

	today := time.Now().UTC().Truncate(24 * time.Hour)

	// Insert a snapshot for today with known values.
	snap := &model.AnalyticsSnapshot{
		UserID:       userID,
		ChannelID:    &channelID,
		Platform:     "instagram",
		SnapshotDate: today,
		Followers:    2000,
		Views:        10000,
		Likes:        500,
		Comments:     100,
		Shares:       50,
	}
	if err := svc.SaveSnapshot(ctx, snap); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	t.Cleanup(func() {
		pool.Exec(context.Background(),
			`DELETE FROM analytics_snapshots WHERE user_id = $1`, userID)
		cleanupChannel(t, pool, channelID)
	})

	// Manually zero out the engagement_rate so we can verify AggregateDaily recalculates.
	_, err := pool.Exec(ctx, `
		UPDATE analytics_snapshots SET engagement_rate = 0
		WHERE user_id = $1 AND snapshot_date = $2
	`, userID, today)
	if err != nil {
		t.Fatalf("zero engagement_rate: %v", err)
	}

	// Run aggregation.
	if err := svc.AggregateDaily(ctx); err != nil {
		t.Fatalf("aggregate daily: %v", err)
	}

	// Verify recalculated engagement_rate.
	dr := model.DateRange{Start: today, End: today.AddDate(0, 0, 1)}
	stats, err := svc.GetPlatformStats(ctx, userID, "instagram", dr)
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(stats))
	}

	// (500+100+50)/10000*100 = 6.5
	expected := 6.5
	if stats[0].EngagementRate != expected {
		t.Errorf("expected engagement_rate %.4f, got %.4f", expected, stats[0].EngagementRate)
	}
}

// Ensure uuid import is used (for growthPct tests that reference uuid.UUID).
var _ = uuid.New
