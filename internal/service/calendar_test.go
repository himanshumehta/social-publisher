package service

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// helper creates a test user + post, returns (userID, postID, cleanup)
func createSchedulablePost(t *testing.T, status string) (context.Context, *CalendarService, *PostService, uuid.UUID, uuid.UUID, func()) {
	t.Helper()
	pool := setupTestPool(t)
	ctx := context.Background()

	userID := uuid.New()
	email := fmt.Sprintf("cal-test-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Calendar Test User", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	postSvc := NewPostService(pool, nil, nil, nil, slog.Default())
	calSvc := NewCalendarService(pool)

	req := &model.CreatePostRequest{
		SourceURL:       fmt.Sprintf("https://example.com/cal-test-%s", uuid.New().String()),
		Title:           "Calendar Test Post",
		TargetPlatforms: []string{"twitter"},
	}

	post, err := postSvc.Create(ctx, userID, req)
	if err != nil {
		t.Fatalf("failed to create test post: %v", err)
	}

	// If requested status is not 'draft', manually set it
	if status != "draft" && status != "" {
		_, err = pool.Exec(ctx,
			`UPDATE posts SET status = $2::post_status WHERE id = $1`,
			post.ID, status,
		)
		if err != nil {
			t.Fatalf("failed to set post status to %s: %v", status, err)
		}
	}

	cleanup := func() {
		pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
		pool.Close()
	}

	return ctx, calSvc, postSvc, userID, post.ID, cleanup
}

// markApproved simulates a post that has passed admin review.
func markApproved(t *testing.T, pool *pgxpool.Pool, postID uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE posts SET status = 'approved'::post_status WHERE id = $1`, postID); err != nil {
		t.Fatalf("approve post: %v", err)
	}
}

func TestSchedulePost_Success(t *testing.T) {
	ctx, calSvc, postSvc, userID, postID, cleanup := createSchedulablePost(t, "approved")
	defer cleanup()

	futureTime := time.Now().UTC().Add(24 * time.Hour)

	err := calSvc.SchedulePost(ctx, userID, postID, futureTime, "America/New_York")
	if err != nil {
		t.Fatalf("SchedulePost failed: %v", err)
	}

	// Verify post status changed
	post, err := postSvc.GetByID(ctx, postID, userID, true)
	if err != nil {
		t.Fatalf("GetByID after schedule failed: %v", err)
	}

	if post.Status != "scheduled" {
		t.Errorf("expected status 'scheduled', got %q", post.Status)
	}
	if post.ScheduledAt == nil {
		t.Fatal("expected scheduled_at to be set")
	}
	if post.Timezone != "America/New_York" {
		t.Errorf("expected timezone 'America/New_York', got %q", post.Timezone)
	}
}

func TestSchedulePost_InvalidStatus(t *testing.T) {
	ctx, calSvc, _, userID, postID, cleanup := createSchedulablePost(t, "published")
	defer cleanup()

	futureTime := time.Now().UTC().Add(24 * time.Hour)

	err := calSvc.SchedulePost(ctx, userID, postID, futureTime, "UTC")
	if err == nil {
		t.Fatal("expected error when scheduling a published post, got nil")
	}
}

func TestSchedulePost_PastTime(t *testing.T) {
	ctx, calSvc, _, userID, postID, cleanup := createSchedulablePost(t, "approved")
	defer cleanup()

	pastTime := time.Now().UTC().Add(-1 * time.Hour)

	err := calSvc.SchedulePost(ctx, userID, postID, pastTime, "UTC")
	if err == nil {
		t.Fatal("expected error when scheduling in the past, got nil")
	}
}

func TestSchedulePost_WrongUser(t *testing.T) {
	ctx, calSvc, _, _, postID, cleanup := createSchedulablePost(t, "approved")
	defer cleanup()

	otherUser := uuid.New()
	futureTime := time.Now().UTC().Add(24 * time.Hour)

	err := calSvc.SchedulePost(ctx, otherUser, postID, futureTime, "UTC")
	if err == nil {
		t.Fatal("expected error when wrong user schedules post, got nil")
	}
}

func TestUnschedulePost_Success(t *testing.T) {
	ctx, calSvc, postSvc, userID, postID, cleanup := createSchedulablePost(t, "approved")
	defer cleanup()

	// Schedule first
	futureTime := time.Now().UTC().Add(24 * time.Hour)
	err := calSvc.SchedulePost(ctx, userID, postID, futureTime, "UTC")
	if err != nil {
		t.Fatalf("SchedulePost failed: %v", err)
	}

	// Unschedule
	err = calSvc.UnschedulePost(ctx, userID, postID)
	if err != nil {
		t.Fatalf("UnschedulePost failed: %v", err)
	}

	post, err := postSvc.GetByID(ctx, postID, userID, true)
	if err != nil {
		t.Fatalf("GetByID after unschedule failed: %v", err)
	}

	// Should revert to pre-schedule status ('approved')
	if post.Status != "approved" {
		t.Errorf("expected status 'approved' after unschedule, got %q", post.Status)
	}
	if post.ScheduledAt != nil {
		t.Errorf("expected scheduled_at to be nil after unschedule, got %v", *post.ScheduledAt)
	}
}

func TestUnschedulePost_NotScheduled(t *testing.T) {
	ctx, calSvc, _, userID, postID, cleanup := createSchedulablePost(t, "approved")
	defer cleanup()

	err := calSvc.UnschedulePost(ctx, userID, postID)
	if err == nil {
		t.Fatal("expected error when unscheduling a non-scheduled post, got nil")
	}
}

func TestBulkSchedule_AllSuccess(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	userID := uuid.New()
	email := fmt.Sprintf("cal-bulk-ok-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Bulk Schedule Test User", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	postSvc := NewPostService(pool, nil, nil, nil, slog.Default())
	calSvc := NewCalendarService(pool)

	// Create 3 posts
	var postIDs []uuid.UUID
	for i := 0; i < 3; i++ {
		req := &model.CreatePostRequest{
			SourceURL:       fmt.Sprintf("https://example.com/bulk-%s-%d", uuid.New().String(), i),
			Title:           fmt.Sprintf("Bulk Post %d", i),
			TargetPlatforms: []string{"twitter"},
		}
		post, err := postSvc.Create(ctx, userID, req)
		if err != nil {
			t.Fatalf("Create post %d failed: %v", i, err)
		}
		markApproved(t, pool, post.ID)
		postIDs = append(postIDs, post.ID)
	}

	// Bulk schedule all
	items := make([]model.BulkScheduleItem, 3)
	for i, pid := range postIDs {
		items[i] = model.BulkScheduleItem{
			PostID:      pid,
			ScheduledAt: time.Now().UTC().Add(time.Duration(i+1) * 24 * time.Hour),
			Timezone:    "UTC",
		}
	}

	results, err := calSvc.BulkSchedule(ctx, userID, items)
	if err != nil {
		t.Fatalf("BulkSchedule failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	for i, r := range results {
		if !r.Success {
			t.Errorf("result[%d] expected success, got error: %s", i, r.Error)
		}
	}
}

func TestBulkSchedule_PartialFailure(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	userID := uuid.New()
	email := fmt.Sprintf("cal-bulk-partial-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Bulk Partial Test User", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	postSvc := NewPostService(pool, nil, nil, nil, slog.Default())
	calSvc := NewCalendarService(pool)

	// Create one schedulable post and one published post
	req1 := &model.CreatePostRequest{
		SourceURL:       fmt.Sprintf("https://example.com/bulk-ok-%s", uuid.New().String()),
		Title:           "Bulk OK Post",
		TargetPlatforms: []string{"twitter"},
	}
	post1, err := postSvc.Create(ctx, userID, req1)
	if err != nil {
		t.Fatalf("Create post1 failed: %v", err)
	}
	markApproved(t, pool, post1.ID)

	req2 := &model.CreatePostRequest{
		SourceURL:       fmt.Sprintf("https://example.com/bulk-fail-%s", uuid.New().String()),
		Title:           "Bulk Fail Post",
		TargetPlatforms: []string{"twitter"},
	}
	post2, err := postSvc.Create(ctx, userID, req2)
	if err != nil {
		t.Fatalf("Create post2 failed: %v", err)
	}

	// Set post2 to published (cannot be scheduled)
	_, err = pool.Exec(ctx,
		`UPDATE posts SET status = 'published'::post_status WHERE id = $1`, post2.ID)
	if err != nil {
		t.Fatalf("failed to set post2 to published: %v", err)
	}

	items := []model.BulkScheduleItem{
		{PostID: post1.ID, ScheduledAt: time.Now().UTC().Add(24 * time.Hour), Timezone: "UTC"},
		{PostID: post2.ID, ScheduledAt: time.Now().UTC().Add(48 * time.Hour), Timezone: "UTC"},
	}

	results, err := calSvc.BulkSchedule(ctx, userID, items)
	if err != nil {
		t.Fatalf("BulkSchedule failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if !results[0].Success {
		t.Errorf("result[0] expected success, got error: %s", results[0].Error)
	}
	if results[1].Success {
		t.Error("result[1] expected failure for published post, got success")
	}
	if results[1].Error == "" {
		t.Error("result[1] expected non-empty error message")
	}
}

func TestGetCalendar_DateRange(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	userID := uuid.New()
	email := fmt.Sprintf("cal-range-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Calendar Range Test User", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	calSvc := NewCalendarService(pool)
	postSvc := NewPostService(pool, nil, nil, nil, slog.Default())

	// Create and schedule two posts on different days
	now := time.Now().UTC()
	day1 := now.Add(24 * time.Hour).Truncate(24 * time.Hour).Add(10 * time.Hour) // tomorrow 10am
	day2 := now.Add(48 * time.Hour).Truncate(24 * time.Hour).Add(14 * time.Hour) // day after tomorrow 2pm

	for i, scheduledAt := range []time.Time{day1, day2} {
		req := &model.CreatePostRequest{
			SourceURL:       fmt.Sprintf("https://example.com/cal-range-%s-%d", uuid.New().String(), i),
			Title:           fmt.Sprintf("Range Post %d", i),
			TargetPlatforms: []string{"twitter"},
		}
		post, err := postSvc.Create(ctx, userID, req)
		if err != nil {
			t.Fatalf("Create post %d failed: %v", i, err)
		}
		markApproved(t, pool, post.ID)
		err = calSvc.SchedulePost(ctx, userID, post.ID, scheduledAt, "UTC")
		if err != nil {
			t.Fatalf("SchedulePost %d failed: %v", i, err)
		}
	}

	// Query the calendar for the range that includes both days
	start := now.Truncate(24 * time.Hour)
	end := now.Add(72 * time.Hour).Truncate(24 * time.Hour).Add(24 * time.Hour)

	entries, err := calSvc.GetCalendar(ctx, userID, start, end)
	if err != nil {
		t.Fatalf("GetCalendar failed: %v", err)
	}

	if len(entries) < 2 {
		t.Errorf("expected at least 2 calendar entries, got %d", len(entries))
	}

	// Verify each entry has posts
	for _, entry := range entries {
		if len(entry.Posts) == 0 {
			t.Errorf("calendar entry for %s has no posts", entry.Date)
		}
		for _, p := range entry.Posts {
			if p.Status != "scheduled" {
				t.Errorf("expected status 'scheduled', got %q", p.Status)
			}
		}
	}
}

func TestGetCalendar_EmptyRange(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	userID := uuid.New()
	email := fmt.Sprintf("cal-empty-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Calendar Empty Test User", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	calSvc := NewCalendarService(pool)

	// Query a range where there are no posts
	start := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2099, 1, 31, 23, 59, 59, 0, time.UTC)

	entries, err := calSvc.GetCalendar(ctx, userID, start, end)
	if err != nil {
		t.Fatalf("GetCalendar failed: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty range, got %d", len(entries))
	}
}

func TestPickDueScheduled_FindsDuePosts(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	userID := uuid.New()
	email := fmt.Sprintf("cal-due-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Due Scheduled Test User", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	postSvc := NewPostService(pool, nil, nil, nil, slog.Default())
	calSvc := NewCalendarService(pool)

	// Create a post and manually set it as scheduled in the past
	req := &model.CreatePostRequest{
		SourceURL:       fmt.Sprintf("https://example.com/due-test-%s", uuid.New().String()),
		Title:           "Due Test Post",
		TargetPlatforms: []string{"twitter"},
	}
	post, err := postSvc.Create(ctx, userID, req)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Manually set status to scheduled with past scheduled_at
	pastTime := time.Now().UTC().Add(-5 * time.Minute)
	_, err = pool.Exec(ctx,
		`UPDATE posts SET status = 'scheduled'::post_status, scheduled_at = $2 WHERE id = $1`,
		post.ID, pastTime,
	)
	if err != nil {
		t.Fatalf("failed to set post as scheduled in past: %v", err)
	}

	ids, err := calSvc.PickDueScheduled(ctx)
	if err != nil {
		t.Fatalf("PickDueScheduled failed: %v", err)
	}

	found := false
	for _, id := range ids {
		if id == post.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected post %s to be in due list, but it was not found", post.ID)
	}
}

func TestPickDueScheduled_IgnoresFuturePosts(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	userID := uuid.New()
	email := fmt.Sprintf("cal-future-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Future Scheduled Test User", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	postSvc := NewPostService(pool, nil, nil, nil, slog.Default())
	calSvc := NewCalendarService(pool)

	// Create a post and schedule it far in the future
	req := &model.CreatePostRequest{
		SourceURL:       fmt.Sprintf("https://example.com/future-test-%s", uuid.New().String()),
		Title:           "Future Test Post",
		TargetPlatforms: []string{"twitter"},
	}
	post, err := postSvc.Create(ctx, userID, req)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	futureTime := time.Now().UTC().Add(72 * time.Hour)
	_, err = pool.Exec(ctx,
		`UPDATE posts SET status = 'scheduled'::post_status, scheduled_at = $2 WHERE id = $1`,
		post.ID, futureTime,
	)
	if err != nil {
		t.Fatalf("failed to set post as scheduled in future: %v", err)
	}

	ids, err := calSvc.PickDueScheduled(ctx)
	if err != nil {
		t.Fatalf("PickDueScheduled failed: %v", err)
	}

	for _, id := range ids {
		if id == post.ID {
			t.Errorf("expected post %s to NOT be in due list (scheduled in future), but it was found", post.ID)
		}
	}
}
