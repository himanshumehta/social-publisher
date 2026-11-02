package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// These tests pin the rule that nothing reaches the publish stage without an
// admin approval: not scheduling, not the schedule worker, not regenerate.
// They need TEST_DATABASE_URL and skip without it.

func newGateFixture(t *testing.T) (context.Context, *pgxpool.Pool, *PostService, *CalendarService, uuid.UUID) {
	t.Helper()
	pool := setupTestPool(t)
	ctx := context.Background()
	userID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, 'Gate Test', $2, 'creator')`,
		userID, fmt.Sprintf("gate-%s@example.com", uuid.New().String()[:8]),
	); err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
		pool.Close()
	})
	return ctx, pool, NewPostService(pool, nil, nil, nil, slog.Default()), NewCalendarService(pool), userID
}

func createPostWithStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, svc *PostService, authorID uuid.UUID, status string) uuid.UUID {
	t.Helper()
	post, err := svc.Create(ctx, authorID, &model.CreatePostRequest{
		Title:           "Gate " + status,
		TargetPlatforms: []string{"linkedin"},
		PlatformPosts:   map[string]*model.PlatformPostEdit{"linkedin": {Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("create post: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE posts SET status = $2::post_status WHERE id = $1`, post.ID, status); err != nil {
		t.Fatalf("set status %s: %v", status, err)
	}
	return post.ID
}

func postStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) string {
	t.Helper()
	var status string
	if err := pool.QueryRow(ctx, `SELECT status::text FROM posts WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	return status
}

func TestSchedulePost_RejectsUnreviewedPosts(t *testing.T) {
	ctx, pool, postSvc, calSvc, userID := newGateFixture(t)
	for _, status := range []string{"draft", "creator_editing", "generated", "pending_review", "flagged"} {
		id := createPostWithStatus(t, ctx, pool, postSvc, userID, status)
		err := calSvc.SchedulePost(ctx, userID, id, time.Now().Add(time.Hour), "UTC")
		if err == nil {
			t.Errorf("status %s: SchedulePost succeeded, want error (would bypass review)", status)
		}
		if got := postStatus(t, ctx, pool, id); got != status {
			t.Errorf("status %s: changed to %s", status, got)
		}
	}
}

func TestTransitionToPublishing_DoesNotReleaseUnreviewed(t *testing.T) {
	ctx, pool, postSvc, calSvc, userID := newGateFixture(t)
	// A row scheduled from an unreviewed status (possible before the fix).
	id := createPostWithStatus(t, ctx, pool, postSvc, userID, "scheduled")
	if _, err := pool.Exec(ctx,
		`UPDATE posts SET scheduled_at = NOW() - INTERVAL '1 minute', pre_schedule_status = 'generated' WHERE id = $1`, id,
	); err != nil {
		t.Fatal(err)
	}

	released, err := calSvc.TransitionToPublishing(ctx, id)
	if err != nil {
		t.Fatalf("TransitionToPublishing: %v", err)
	}
	if released {
		t.Error("released = true for a post that was never approved")
	}
	if got := postStatus(t, ctx, pool, id); got != "generated" {
		t.Errorf("status = %s, want generated", got)
	}
}

func TestAdminApprove_WithSchedule(t *testing.T) {
	ctx, pool, postSvc, calSvc, userID := newGateFixture(t)
	id := createPostWithStatus(t, ctx, pool, postSvc, userID, "pending_review")

	past := time.Now().Add(-time.Minute)
	if err := postSvc.AdminApprove(ctx, id, &past); err == nil {
		t.Error("AdminApprove accepted a scheduled_at in the past")
	}

	at := time.Now().Add(time.Hour)
	if err := postSvc.AdminApprove(ctx, id, &at); err != nil {
		t.Fatalf("AdminApprove: %v", err)
	}
	if got := postStatus(t, ctx, pool, id); got != "scheduled" {
		t.Fatalf("status = %s, want scheduled", got)
	}

	// Make it due and let the schedule worker release it.
	if _, err := pool.Exec(ctx, `UPDATE posts SET scheduled_at = NOW() - INTERVAL '1 second' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	released, err := calSvc.TransitionToPublishing(ctx, id)
	if err != nil || !released {
		t.Fatalf("TransitionToPublishing = %v, %v; want true, nil", released, err)
	}
	if got := postStatus(t, ctx, pool, id); got != "approved" {
		t.Errorf("status = %s, want approved", got)
	}
}

func TestRegenerate_RequiresOwnershipOrAdmin(t *testing.T) {
	ctx, pool, postSvc, _, ownerID := newGateFixture(t)
	id := createPostWithStatus(t, ctx, pool, postSvc, ownerID, "generated")

	if err := postSvc.Regenerate(ctx, id, uuid.New(), false); err == nil {
		t.Error("another user regenerated the post")
	}
	if got := postStatus(t, ctx, pool, id); got != "generated" {
		t.Fatalf("status changed to %s by non-owner", got)
	}
	if err := postSvc.Regenerate(ctx, id, ownerID, false); err != nil {
		t.Errorf("owner regenerate: %v", err)
	}
}

func TestSubmit_RunsAutomatedReview(t *testing.T) {
	ctx, pool, _, _, userID := newGateFixture(t)
	svc := NewPostService(pool, nil, nil, NewPostReviewer(slog.Default()), slog.Default())
	post, err := svc.Create(ctx, userID, &model.CreatePostRequest{
		Title:           "Too long",
		TargetPlatforms: []string{"twitter"},
		PlatformPosts:   map[string]*model.PlatformPostEdit{"twitter": {Text: strings.Repeat("a", 300)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Submit(ctx, post.ID, userID); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	got, err := svc.GetByID(ctx, post.ID, userID, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "pending_review" || !strings.Contains(got.SystemReviewNotes, "char_limit_exceeded") {
		t.Fatalf("status %q, notes %q: hand-written copy skipped the automated review", got.Status, got.SystemReviewNotes)
	}
}
