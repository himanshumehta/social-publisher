package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ---------------------------------------------------------------------------
// Transition-specific test fixture helpers
//
// Shared helpers (createTestProject, createTestVideo, createTestChannel,
// findUserByRole, cleanupProject, cleanupChannel) are defined in
// test_helpers_test.go. findTwoDistinctUsers is in channel_target_test.go.
// ---------------------------------------------------------------------------

// createTestChannelTarget inserts a channel_target row with a video reference.
func createTestChannelTarget(t *testing.T, pool *pgxpool.Pool, projectID, videoID, channelID, createdBy uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(),
		`INSERT INTO channel_targets (project_id, video_id, channel_id, created_by)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id`,
		projectID, videoID, channelID, createdBy,
	).Scan(&id)
	if err != nil {
		t.Fatalf("failed to create test channel target: %v", err)
	}
	return id
}

// ---------------------------------------------------------------------------
// Tests — Simplified 4-status model: active → in_review → published → archived
// ---------------------------------------------------------------------------

func TestTransitionService_ActiveToInReview(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	svc := NewTransitionService(pool)

	creatorID := findUserByRole(t, pool, "creator")

	t.Run("fails_without_video", func(t *testing.T) {
		projectID := createTestProject(t, pool, creatorID, string(model.ProjectActive))
		defer cleanupProject(t, pool, projectID)

		_, err := svc.Transition(ctx, creatorID, "creator", projectID, TransitionRequest{
			ToStatus: "in_review",
			Comment:  "Submit for review",
		})
		if err == nil {
			t.Fatal("expected error when no video exists")
		}
		if !strings.Contains(err.Error(), "video") {
			t.Errorf("expected error about video, got: %v", err)
		}
	})

	t.Run("fails_without_channel_target", func(t *testing.T) {
		projectID := createTestProject(t, pool, creatorID, string(model.ProjectActive))
		defer cleanupProject(t, pool, projectID)

		createTestVideo(t, pool, projectID, creatorID, "youtube_16_9")

		_, err := svc.Transition(ctx, creatorID, "creator", projectID, TransitionRequest{
			ToStatus: "in_review",
			Comment:  "Submit for review",
		})
		if err == nil {
			t.Fatal("expected error when no channel target exists")
		}
		if !strings.Contains(err.Error(), "channel target") {
			t.Errorf("expected error about channel target, got: %v", err)
		}
	})

	t.Run("succeeds_with_video_and_channel_target", func(t *testing.T) {
		projectID := createTestProject(t, pool, creatorID, string(model.ProjectActive))
		defer cleanupProject(t, pool, projectID)

		channelID := createTestChannel(t, pool)
		defer cleanupChannel(t, pool, channelID)

		videoID := createTestVideo(t, pool, projectID, creatorID, "youtube_16_9")
		createTestChannelTarget(t, pool, projectID, videoID, channelID, creatorID)

		project, err := svc.Transition(ctx, creatorID, "creator", projectID, TransitionRequest{
			ToStatus: "in_review",
			Comment:  "Ready for admin review",
		})
		if err != nil {
			t.Fatalf("transition active -> in_review failed: %v", err)
		}
		if project.Status != model.ProjectInReview {
			t.Errorf("expected status in_review, got %q", project.Status)
		}
	})
}

func TestTransitionService_InReviewTransitions(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	svc := NewTransitionService(pool)

	creatorID := findUserByRole(t, pool, "creator")
	adminID := findUserByRole(t, pool, "admin")

	t.Run("admin_can_return_to_active", func(t *testing.T) {
		projectID := createTestProject(t, pool, creatorID, string(model.ProjectInReview))
		defer cleanupProject(t, pool, projectID)

		project, err := svc.Transition(ctx, adminID, "admin", projectID, TransitionRequest{
			ToStatus: "active",
			Comment:  "Needs changes",
		})
		if err != nil {
			t.Fatalf("transition in_review -> active failed: %v", err)
		}
		if project.Status != model.ProjectActive {
			t.Errorf("expected status active, got %q", project.Status)
		}
	})

	t.Run("admin_can_publish_with_channel_targets", func(t *testing.T) {
		projectID := createTestProject(t, pool, creatorID, string(model.ProjectInReview))
		defer cleanupProject(t, pool, projectID)

		channelID := createTestChannel(t, pool)
		defer cleanupChannel(t, pool, channelID)

		videoID := createTestVideo(t, pool, projectID, creatorID, "youtube_16_9")
		createTestChannelTarget(t, pool, projectID, videoID, channelID, creatorID)

		project, err := svc.Transition(ctx, adminID, "admin", projectID, TransitionRequest{
			ToStatus: "published",
			Comment:  "Approved",
		})
		if err != nil {
			t.Fatalf("transition in_review -> published failed: %v", err)
		}
		if project.Status != model.ProjectPublished {
			t.Errorf("expected status published, got %q", project.Status)
		}
	})

	t.Run("publish_fails_without_channel_targets", func(t *testing.T) {
		projectID := createTestProject(t, pool, creatorID, string(model.ProjectInReview))
		defer cleanupProject(t, pool, projectID)

		_, err := svc.Transition(ctx, adminID, "admin", projectID, TransitionRequest{
			ToStatus: "published",
			Comment:  "Try to publish",
		})
		if err == nil {
			t.Fatal("expected error when no channel targets exist")
		}
		if !strings.Contains(err.Error(), "channel target") {
			t.Errorf("expected error about channel targets, got: %v", err)
		}
	})
}

func TestTransitionService_PublishedToArchived(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	svc := NewTransitionService(pool)

	creatorID := findUserByRole(t, pool, "creator")
	adminID := findUserByRole(t, pool, "admin")

	t.Run("admin_can_archive_published_project", func(t *testing.T) {
		projectID := createTestProject(t, pool, creatorID, string(model.ProjectPublished))
		defer cleanupProject(t, pool, projectID)

		project, err := svc.Transition(ctx, adminID, "admin", projectID, TransitionRequest{
			ToStatus: "archived",
			Comment:  "Archiving project",
		})
		if err != nil {
			t.Fatalf("transition published -> archived failed: %v", err)
		}
		if project.Status != model.ProjectArchived {
			t.Errorf("expected status archived, got %q", project.Status)
		}
	})

	t.Run("admin_can_archive_from_any_status", func(t *testing.T) {
		projectID := createTestProject(t, pool, creatorID, string(model.ProjectActive))
		defer cleanupProject(t, pool, projectID)

		project, err := svc.Transition(ctx, adminID, "admin", projectID, TransitionRequest{
			ToStatus: "archived",
			Comment:  "Archiving active project",
		})
		if err != nil {
			t.Fatalf("transition active -> archived failed: %v", err)
		}
		if project.Status != model.ProjectArchived {
			t.Errorf("expected status archived, got %q", project.Status)
		}
	})
}

func TestTransitionService_RoleValidation(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	svc := NewTransitionService(pool)

	creatorID := findUserByRole(t, pool, "creator")

	t.Run("creator_cannot_publish", func(t *testing.T) {
		projectID := createTestProject(t, pool, creatorID, string(model.ProjectInReview))
		defer cleanupProject(t, pool, projectID)

		_, err := svc.Transition(ctx, creatorID, "creator", projectID, TransitionRequest{
			ToStatus: "published",
			Comment:  "Creator tries to publish",
		})
		if err == nil {
			t.Fatal("expected error: creator should not be allowed to publish (admin-only)")
		}
		if !strings.Contains(err.Error(), "role") {
			t.Errorf("expected role-related error, got: %v", err)
		}
	})

	t.Run("creator_cannot_archive", func(t *testing.T) {
		projectID := createTestProject(t, pool, creatorID, string(model.ProjectActive))
		defer cleanupProject(t, pool, projectID)

		_, err := svc.Transition(ctx, creatorID, "creator", projectID, TransitionRequest{
			ToStatus: "archived",
			Comment:  "Creator tries to archive",
		})
		if err == nil {
			t.Fatal("expected error: creator should not be allowed to archive (admin-only)")
		}
	})

	t.Run("non_owner_creator_cannot_submit_for_review", func(t *testing.T) {
		projectID := createTestProject(t, pool, creatorID, string(model.ProjectActive))
		defer cleanupProject(t, pool, projectID)

		channelID := createTestChannel(t, pool)
		defer cleanupChannel(t, pool, channelID)
		videoID := createTestVideo(t, pool, projectID, creatorID, "youtube_16_9")
		createTestChannelTarget(t, pool, projectID, videoID, channelID, creatorID)

		// A random user ID (not the creator) tries the transition.
		_, err := svc.Transition(ctx, uuid.New(), "creator", projectID, TransitionRequest{
			ToStatus: "in_review",
			Comment:  "Wrong creator tries",
		})
		if err == nil {
			t.Fatal("expected error: non-owner creator should be blocked by ownership check")
		}
		if !strings.Contains(err.Error(), "not the creator") {
			t.Errorf("expected ownership error, got: %v", err)
		}
	})
}

func TestTransitionService_GetAllowedTransitions(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	svc := NewTransitionService(pool)

	creatorID := findUserByRole(t, pool, "creator")
	adminID := findUserByRole(t, pool, "admin")

	t.Run("active_creator_can_submit_for_review", func(t *testing.T) {
		projectID := createTestProject(t, pool, creatorID, string(model.ProjectActive))
		defer cleanupProject(t, pool, projectID)

		allowed, err := svc.GetAllowedTransitions(ctx, projectID, creatorID, "creator")
		if err != nil {
			t.Fatalf("GetAllowedTransitions failed: %v", err)
		}

		hasInReview := false
		for _, s := range allowed {
			if s == "in_review" {
				hasInReview = true
			}
		}
		if !hasInReview {
			t.Errorf("active project should allow in_review for creator, got: %v", allowed)
		}
	})

	t.Run("in_review_admin_can_publish_and_return", func(t *testing.T) {
		projectID := createTestProject(t, pool, creatorID, string(model.ProjectInReview))
		defer cleanupProject(t, pool, projectID)

		allowed, err := svc.GetAllowedTransitions(ctx, projectID, adminID, "admin")
		if err != nil {
			t.Fatalf("GetAllowedTransitions failed: %v", err)
		}

		hasPublished := false
		hasActive := false
		hasArchived := false
		for _, s := range allowed {
			switch s {
			case "published":
				hasPublished = true
			case "active":
				hasActive = true
			case "archived":
				hasArchived = true
			}
		}
		if !hasPublished {
			t.Errorf("admin should be able to publish from in_review, got: %v", allowed)
		}
		if !hasActive {
			t.Errorf("admin should be able to return to active from in_review, got: %v", allowed)
		}
		if !hasArchived {
			t.Errorf("admin should be able to archive from in_review, got: %v", allowed)
		}
	})

	t.Run("published_admin_can_archive", func(t *testing.T) {
		projectID := createTestProject(t, pool, creatorID, string(model.ProjectPublished))
		defer cleanupProject(t, pool, projectID)

		allowed, err := svc.GetAllowedTransitions(ctx, projectID, adminID, "admin")
		if err != nil {
			t.Fatalf("GetAllowedTransitions failed: %v", err)
		}

		hasArchived := false
		for _, s := range allowed {
			if s == "archived" {
				hasArchived = true
			}
		}
		if !hasArchived {
			t.Errorf("admin should be able to archive published project, got: %v", allowed)
		}
	})

	t.Run("archived_has_no_transitions", func(t *testing.T) {
		projectID := createTestProject(t, pool, creatorID, string(model.ProjectArchived))
		defer cleanupProject(t, pool, projectID)

		allowed, err := svc.GetAllowedTransitions(ctx, projectID, creatorID, "creator")
		if err != nil {
			t.Fatalf("GetAllowedTransitions failed: %v", err)
		}
		if len(allowed) != 0 {
			t.Errorf("archived status should have no transitions for creator, got: %v", allowed)
		}
	})
}

func TestTransitionService_AutoPublish(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	svc := NewTransitionService(pool)

	creatorID := findUserByRole(t, pool, "creator")
	adminID := findUserByRole(t, pool, "admin")

	t.Run("publish_creates_publish_records_and_deletes_targets", func(t *testing.T) {
		projectID := createTestProject(t, pool, creatorID, string(model.ProjectInReview))
		defer cleanupProject(t, pool, projectID)

		channelID1 := createTestChannel(t, pool)
		defer cleanupChannel(t, pool, channelID1)
		channelID2 := createTestChannel(t, pool)
		defer cleanupChannel(t, pool, channelID2)

		videoID := createTestVideo(t, pool, projectID, creatorID, "youtube_16_9")
		createTestChannelTarget(t, pool, projectID, videoID, channelID1, creatorID)

		videoID2 := createTestVideo(t, pool, projectID, creatorID, "instagram_reel")
		createTestChannelTarget(t, pool, projectID, videoID2, channelID2, creatorID)

		project, err := svc.Transition(ctx, adminID, "admin", projectID, TransitionRequest{
			ToStatus: "published",
			Comment:  "Publishing with auto-publish",
		})
		if err != nil {
			t.Fatalf("transition in_review -> published failed: %v", err)
		}
		if project.Status != model.ProjectPublished {
			t.Errorf("expected status published, got %q", project.Status)
		}

		// Verify publish_records were created.
		var publishCount int
		err = pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM publish_records WHERE project_id = $1`, projectID,
		).Scan(&publishCount)
		if err != nil {
			t.Fatalf("failed to count publish records: %v", err)
		}
		if publishCount != 2 {
			t.Errorf("expected 2 publish records, got %d", publishCount)
		}

		// Verify publish_records have correct attributes.
		var visibility, status string
		err = pool.QueryRow(ctx,
			`SELECT visibility, status FROM publish_records WHERE project_id = $1 LIMIT 1`, projectID,
		).Scan(&visibility, &status)
		if err != nil {
			t.Fatalf("failed to query publish record: %v", err)
		}
		if visibility != "public" {
			t.Errorf("expected visibility 'public', got %q", visibility)
		}
		if status != "pending" {
			t.Errorf("expected publish record status 'pending', got %q", status)
		}

		// Verify channel_targets were deleted.
		var targetCount int
		err = pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM channel_targets WHERE project_id = $1`, projectID,
		).Scan(&targetCount)
		if err != nil {
			t.Fatalf("failed to count channel targets: %v", err)
		}
		if targetCount != 0 {
			t.Errorf("expected 0 channel targets after auto-publish, got %d", targetCount)
		}
	})
}

func TestTransitionService_InvalidTransitions(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	svc := NewTransitionService(pool)

	creatorID := findUserByRole(t, pool, "creator")

	t.Run("project_not_found", func(t *testing.T) {
		_, err := svc.Transition(ctx, creatorID, "creator", uuid.New(), TransitionRequest{
			ToStatus: "in_review",
		})
		if err == nil {
			t.Fatal("expected error for non-existent project")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected not found error, got: %v", err)
		}
	})

	t.Run("get_allowed_transitions_project_not_found", func(t *testing.T) {
		_, err := svc.GetAllowedTransitions(ctx, uuid.New(), creatorID, "creator")
		if err == nil {
			t.Fatal("expected error for non-existent project")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected not found error, got: %v", err)
		}
	})

	t.Run("in_review_to_in_review_is_invalid", func(t *testing.T) {
		projectID := createTestProject(t, pool, creatorID, string(model.ProjectInReview))
		defer cleanupProject(t, pool, projectID)

		adminID := findUserByRole(t, pool, "admin")
		_, err := svc.Transition(ctx, adminID, "admin", projectID, TransitionRequest{
			ToStatus: "in_review",
		})
		if err == nil {
			t.Fatal("expected error: in_review -> in_review is not a valid transition")
		}
	})

	t.Run("published_to_active_is_invalid", func(t *testing.T) {
		projectID := createTestProject(t, pool, creatorID, string(model.ProjectPublished))
		defer cleanupProject(t, pool, projectID)

		adminID := findUserByRole(t, pool, "admin")
		_, err := svc.Transition(ctx, adminID, "admin", projectID, TransitionRequest{
			ToStatus: "active",
		})
		if err == nil {
			t.Fatal("expected error: published -> active is not a valid transition")
		}
	})
}

func TestTransitionService_ReviewAuditTrail(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	svc := NewTransitionService(pool)

	creatorID := findUserByRole(t, pool, "creator")
	adminID := findUserByRole(t, pool, "admin")

	projectID := createTestProject(t, pool, creatorID, string(model.ProjectInReview))
	defer cleanupProject(t, pool, projectID)

	_, err := svc.Transition(ctx, adminID, "admin", projectID, TransitionRequest{
		ToStatus: "active",
		Comment:  "Returning to active for review audit test",
	})
	if err != nil {
		t.Fatalf("transition failed: %v", err)
	}

	// Verify a review row was inserted.
	var reviewCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM reviews WHERE project_id = $1 AND action = 'transition'`, projectID,
	).Scan(&reviewCount)
	if err != nil {
		t.Fatalf("failed to count reviews: %v", err)
	}
	if reviewCount == 0 {
		t.Error("expected at least one transition review row")
	}

	// Verify the review has correct from/to status.
	var fromStatus, toStatus, comment string
	err = pool.QueryRow(ctx,
		`SELECT from_status, to_status, COALESCE(comment, '')
		 FROM reviews WHERE project_id = $1 AND action = 'transition'
		 ORDER BY created_at DESC LIMIT 1`, projectID,
	).Scan(&fromStatus, &toStatus, &comment)
	if err != nil {
		t.Fatalf("failed to query review: %v", err)
	}
	if fromStatus != "in_review" {
		t.Errorf("expected from_status 'in_review', got %q", fromStatus)
	}
	if toStatus != "active" {
		t.Errorf("expected to_status 'active', got %q", toStatus)
	}
	if comment != "Returning to active for review audit test" {
		t.Errorf("expected comment to be preserved, got %q", comment)
	}
}
