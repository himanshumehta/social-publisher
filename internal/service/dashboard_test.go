package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

func TestDashboardGetCreatorStats(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	email := fmt.Sprintf("test-dash-creator-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Dashboard Creator Test", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	projectID := createTestProject(t, pool, userID, "active")

	defer cleanupProject(t, pool, projectID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewDashboardService(pool)
	stats, err := svc.GetCreatorStats(ctx, userID)
	if err != nil {
		t.Fatalf("GetCreatorStats failed: %v", err)
	}

	if stats.TotalProjects != 1 {
		t.Errorf("expected TotalProjects=1, got %d", stats.TotalProjects)
	}
	if stats.TotalViews != 0 {
		t.Errorf("expected TotalViews=0 for new project, got %d", stats.TotalViews)
	}
	if stats.TotalEarnings != 0 {
		t.Errorf("expected TotalEarnings=0 for new project, got %d", stats.TotalEarnings)
	}
	if stats.TotalVideos != 0 {
		t.Errorf("expected TotalVideos=0 for new project, got %d", stats.TotalVideos)
	}
	if stats.Recent == nil {
		t.Error("expected Recent to be non-nil slice")
	}
	if len(stats.Recent) != 1 {
		t.Errorf("expected 1 recent activity entry, got %d", len(stats.Recent))
	}
}

func TestDashboardGetCreatorStats_NoData(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	email := fmt.Sprintf("test-dash-nodata-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Dashboard No Data User", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewDashboardService(pool)
	stats, err := svc.GetCreatorStats(ctx, userID)
	if err != nil {
		t.Fatalf("GetCreatorStats (no data) failed: %v", err)
	}

	if stats.TotalProjects != 0 {
		t.Errorf("expected TotalProjects=0, got %d", stats.TotalProjects)
	}
	if stats.Published != 0 {
		t.Errorf("expected Published=0, got %d", stats.Published)
	}
	if stats.NeedsAction != 0 {
		t.Errorf("expected NeedsAction=0, got %d", stats.NeedsAction)
	}
	if stats.TotalVideos != 0 {
		t.Errorf("expected TotalVideos=0, got %d", stats.TotalVideos)
	}
	if stats.TotalViews != 0 {
		t.Errorf("expected TotalViews=0, got %d", stats.TotalViews)
	}
	if stats.TotalEarnings != 0 {
		t.Errorf("expected TotalEarnings=0, got %d", stats.TotalEarnings)
	}
	if len(stats.Recent) != 0 {
		t.Errorf("expected 0 recent activity entries, got %d", len(stats.Recent))
	}
}

func TestDashboardGetAdminStats(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	svc := NewDashboardService(pool)

	stats, err := svc.GetAdminStats(ctx)
	if err != nil {
		t.Fatalf("GetAdminStats failed: %v", err)
	}

	if stats == nil {
		t.Fatal("expected non-nil AdminDashboardStats")
	}
}

func TestDashboardGetAdminStats_Fields(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	svc := NewDashboardService(pool)

	stats, err := svc.GetAdminStats(ctx)
	if err != nil {
		t.Fatalf("GetAdminStats failed: %v", err)
	}

	if stats.TotalCreators < 0 {
		t.Errorf("expected TotalCreators >= 0, got %d", stats.TotalCreators)
	}
	if stats.TotalEditors < 0 {
		t.Errorf("expected TotalEditors >= 0, got %d", stats.TotalEditors)
	}
	if stats.ActiveProjects < 0 {
		t.Errorf("expected ActiveProjects >= 0, got %d", stats.ActiveProjects)
	}
	if stats.PendingReview < 0 {
		t.Errorf("expected PendingReview >= 0, got %d", stats.PendingReview)
	}
	if stats.PendingBanking < 0 {
		t.Errorf("expected PendingBanking >= 0, got %d", stats.PendingBanking)
	}
	if stats.TotalChannels < 0 {
		t.Errorf("expected TotalChannels >= 0, got %d", stats.TotalChannels)
	}
	if stats.TotalViews < 0 {
		t.Errorf("expected TotalViews >= 0, got %d", stats.TotalViews)
	}
	if stats.TotalRevenue < 0 {
		t.Errorf("expected TotalRevenue >= 0, got %d", stats.TotalRevenue)
	}
}
