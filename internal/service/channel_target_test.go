package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ---------------------------------------------------------------------------
// Helpers (unique to channel_target tests)
//
// Shared helpers (createTestProject, createTestVideo, createTestChannel,
// findUserByRole, cleanupProject, cleanupChannel) are defined in
// test_helpers_test.go.
// ---------------------------------------------------------------------------

// findTwoDistinctUsers returns two different user IDs. Skips if fewer than two exist.
func findTwoDistinctUsers(t *testing.T, pool *pgxpool.Pool) (uuid.UUID, uuid.UUID) {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT id FROM users LIMIT 2`)
	if err != nil {
		t.Skipf("skipping: cannot query users: %v", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan user id: %v", err)
		}
		ids = append(ids, id)
	}
	if len(ids) < 2 {
		t.Skipf("skipping: need at least 2 users, found %d", len(ids))
	}
	return ids[0], ids[1]
}

// ---------------------------------------------------------------------------
// ChannelTargetService Tests
// ---------------------------------------------------------------------------

func TestChannelTargetService_CreateBulk(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	creatorID := findUserByRole(t, pool, "creator")

	projectID := createTestProject(t, pool, creatorID, "active")
	videoID := createTestVideo(t, pool, projectID, creatorID, "youtube_16_9")
	channelID := createTestChannel(t, pool)

	defer func() {
		cleanupProject(t, pool, projectID)
		cleanupChannel(t, pool, channelID)
	}()

	svc := NewChannelTargetService(pool)

	now := time.Now().Truncate(time.Second)
	targets := []CreateChannelTargetRequest{
		{
			VideoID:     &videoID,
			ChannelID:   channelID,
			ScheduledAt: &now,
		},
	}

	results, err := svc.CreateBulk(ctx, projectID, creatorID, false, targets)
	if err != nil {
		t.Fatalf("CreateBulk failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]

	// Verify IDs.
	if r.ID == uuid.Nil {
		t.Error("expected channel target to have an ID")
	}
	if r.ProjectID != projectID {
		t.Errorf("expected project_id %s, got %s", projectID, r.ProjectID)
	}
	if r.VideoID == nil || *r.VideoID != videoID {
		t.Errorf("expected video_id %s, got %v", videoID, r.VideoID)
	}
	if r.ChannelID != channelID {
		t.Errorf("expected channel_id %s, got %s", channelID, r.ChannelID)
	}
	if r.CreatedBy != creatorID {
		t.Errorf("expected created_by %s, got %s", creatorID, r.CreatedBy)
	}

	// Verify channel info populated.
	if r.ChannelName == "" {
		t.Error("expected channel_name to be populated")
	}
	if r.Platform != "youtube" {
		t.Errorf("expected platform 'youtube', got %q", r.Platform)
	}

	// Verify scheduled_at.
	if r.ScheduledAt == nil {
		t.Error("expected scheduled_at to be set")
	}
}

func TestChannelTargetService_CreateBulk_Authorization(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	creatorID, otherUserID := findTwoDistinctUsers(t, pool)
	adminID := findUserByRole(t, pool, "admin")

	projectID := createTestProject(t, pool, creatorID, "active")
	videoID := createTestVideo(t, pool, projectID, creatorID, "youtube_16_9")
	channelID := createTestChannel(t, pool)

	defer func() {
		cleanupProject(t, pool, projectID)
		cleanupChannel(t, pool, channelID)
	}()

	svc := NewChannelTargetService(pool)

	t.Run("non-creator non-admin is forbidden", func(t *testing.T) {
		targets := []CreateChannelTargetRequest{
			{VideoID: &videoID, ChannelID: channelID},
		}
		_, err := svc.CreateBulk(ctx, projectID, otherUserID, false, targets)
		if err == nil {
			t.Fatal("expected forbidden error for non-creator, non-admin user")
		}
		if !strings.Contains(err.Error(), "forbidden") {
			t.Errorf("expected 'forbidden' in error, got: %v", err)
		}
	})

	t.Run("admin can create even if not creator", func(t *testing.T) {
		// Use a second video to avoid unique constraint on (video_id, channel_id).
		vid2 := createTestVideo(t, pool, projectID, creatorID, "instagram_reel")
		targets := []CreateChannelTargetRequest{
			{VideoID: &vid2, ChannelID: channelID},
		}
		results, err := svc.CreateBulk(ctx, projectID, adminID, true, targets)
		if err != nil {
			t.Fatalf("admin CreateBulk failed: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
	})

	t.Run("project creator can create", func(t *testing.T) {
		targets := []CreateChannelTargetRequest{
			{VideoID: &videoID, ChannelID: channelID},
		}
		results, err := svc.CreateBulk(ctx, projectID, creatorID, false, targets)
		if err != nil {
			t.Fatalf("creator CreateBulk failed: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
	})
}

func TestChannelTargetService_ListByProject(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	creatorID := findUserByRole(t, pool, "creator")

	projectID := createTestProject(t, pool, creatorID, "active")
	videoID1 := createTestVideo(t, pool, projectID, creatorID, "youtube_16_9")
	videoID2 := createTestVideo(t, pool, projectID, creatorID, "instagram_reel")
	channelID1 := createTestChannel(t, pool)
	channelID2 := createTestChannel(t, pool)

	defer func() {
		cleanupProject(t, pool, projectID)
		cleanupChannel(t, pool, channelID1)
		cleanupChannel(t, pool, channelID2)
	}()

	svc := NewChannelTargetService(pool)

	// Create two channel targets, each with a different video+channel pair.
	targets := []CreateChannelTargetRequest{
		{VideoID: &videoID1, ChannelID: channelID1},
		{VideoID: &videoID2, ChannelID: channelID2},
	}
	created, err := svc.CreateBulk(ctx, projectID, creatorID, false, targets)
	if err != nil {
		t.Fatalf("CreateBulk failed: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("expected 2 created targets, got %d", len(created))
	}

	// List targets for the project.
	listed, err := svc.ListByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("ListByProject failed: %v", err)
	}

	if len(listed) != 2 {
		t.Fatalf("expected 2 listed targets, got %d", len(listed))
	}

	// Verify ordering is created_at DESC (most recent first).
	if listed[0].CreatedAt.Before(listed[1].CreatedAt) {
		t.Error("expected listed targets to be ordered by created_at DESC")
	}

	// Verify channel info is included on each result.
	for i, ct := range listed {
		if ct.ChannelName == "" {
			t.Errorf("listed[%d]: expected channel_name to be populated", i)
		}
		if ct.Platform == "" {
			t.Errorf("listed[%d]: expected platform to be populated", i)
		}
		if ct.ProjectID != projectID {
			t.Errorf("listed[%d]: expected project_id %s, got %s", i, projectID, ct.ProjectID)
		}
	}
}

func TestChannelTargetService_Delete(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	creatorID, otherUserID := findTwoDistinctUsers(t, pool)
	adminID := findUserByRole(t, pool, "admin")

	projectID := createTestProject(t, pool, creatorID, "active")
	channelID := createTestChannel(t, pool)

	defer func() {
		cleanupProject(t, pool, projectID)
		cleanupChannel(t, pool, channelID)
	}()

	svc := NewChannelTargetService(pool)

	// videoCounter tracks how many videos we create so each has a unique format,
	// avoiding the unique constraint on (video_id, channel_id).
	videoCounter := 0
	formats := []string{"youtube_16_9", "youtube_short", "instagram_reel", "facebook_feed", "facebook_reel"}

	// Helper to create a single channel target and return its ID.
	createOne := func(t *testing.T) uuid.UUID {
		t.Helper()
		format := formats[videoCounter%len(formats)]
		videoCounter++
		vid := createTestVideo(t, pool, projectID, creatorID, format)
		results, err := svc.CreateBulk(ctx, projectID, creatorID, false, []CreateChannelTargetRequest{
			{VideoID: &vid, ChannelID: channelID},
		})
		if err != nil {
			t.Fatalf("CreateBulk for delete test: %v", err)
		}
		return results[0].ID
	}

	t.Run("creator can delete own target", func(t *testing.T) {
		targetID := createOne(t)
		if err := svc.Delete(ctx, targetID, creatorID, false); err != nil {
			t.Fatalf("Delete by creator failed: %v", err)
		}

		// Verify it is gone.
		listed, err := svc.ListByProject(ctx, projectID)
		if err != nil {
			t.Fatalf("ListByProject after delete: %v", err)
		}
		for _, ct := range listed {
			if ct.ID == targetID {
				t.Error("expected deleted target to be absent from list")
			}
		}
	})

	t.Run("admin can delete any target", func(t *testing.T) {
		targetID := createOne(t)
		if err := svc.Delete(ctx, targetID, adminID, true); err != nil {
			t.Fatalf("Delete by admin failed: %v", err)
		}
	})

	t.Run("unauthorized user cannot delete", func(t *testing.T) {
		targetID := createOne(t)
		err := svc.Delete(ctx, targetID, otherUserID, false)
		if err == nil {
			t.Fatal("expected forbidden error for unauthorized user")
		}
		if !strings.Contains(err.Error(), "forbidden") {
			t.Errorf("expected 'forbidden' in error, got: %v", err)
		}
	})

	t.Run("delete non-existent target", func(t *testing.T) {
		err := svc.Delete(ctx, uuid.New(), creatorID, false)
		if err == nil {
			t.Error("expected error deleting non-existent target")
		}
	})
}

func TestChannelTargetService_CreateBulk_EmptyTargets(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	creatorID := findUserByRole(t, pool, "creator")
	projectID := createTestProject(t, pool, creatorID, "active")

	defer func() {
		cleanupProject(t, pool, projectID)
	}()

	svc := NewChannelTargetService(pool)

	_, err := svc.CreateBulk(ctx, projectID, creatorID, false, []CreateChannelTargetRequest{})
	if err == nil {
		t.Fatal("expected error when targets slice is empty")
	}
	if !strings.Contains(err.Error(), "at least one target") {
		t.Errorf("expected 'at least one target' in error, got: %v", err)
	}
}
