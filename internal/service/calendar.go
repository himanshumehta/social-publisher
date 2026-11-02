package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// CalendarService handles post scheduling and calendar views.
type CalendarService struct {
	pool *pgxpool.Pool
}

// NewCalendarService creates a new CalendarService.
func NewCalendarService(pool *pgxpool.Pool) *CalendarService {
	return &CalendarService{pool: pool}
}

// schedulableStatuses are the post statuses from which scheduling is allowed.
var schedulableStatuses = map[string]bool{
	"draft":     true,
	"generated": true,
	"approved":  true,
}

// GetCalendar returns posts grouped by date within the given time range.
// Includes posts that are scheduled or published within the range.
func (s *CalendarService) GetCalendar(ctx context.Context, userID uuid.UUID, start, end time.Time) ([]model.CalendarEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, title, status, target_platforms, scheduled_at, published_at, cover_image_url
		FROM posts
		WHERE author_id = $1
		  AND (
			(status = 'scheduled' AND scheduled_at >= $2 AND scheduled_at < $3)
			OR (status = 'published' AND published_at >= $2 AND published_at < $3)
			OR (status = 'publishing' AND scheduled_at >= $2 AND scheduled_at < $3)
		  )
		ORDER BY COALESCE(scheduled_at, published_at) ASC`,
		userID, start, end,
	)
	if err != nil {
		return nil, fmt.Errorf("query calendar posts: %w", err)
	}
	defer rows.Close()

	// Group posts by date string
	dateMap := make(map[string][]model.CalendarPostItem)
	dateOrder := []string{} // maintain insertion order

	for rows.Next() {
		var item model.CalendarPostItem
		var coverImageURL *string

		err := rows.Scan(
			&item.ID, &item.Title, &item.Status,
			&item.TargetPlatforms, &item.ScheduledAt,
			&item.PublishedAt, &coverImageURL,
		)
		if err != nil {
			return nil, fmt.Errorf("scan calendar post: %w", err)
		}
		if coverImageURL != nil {
			item.CoverImageURL = *coverImageURL
		}

		// Determine the date key from scheduled_at or published_at
		var dateKey string
		if item.ScheduledAt != nil {
			dateKey = item.ScheduledAt.Format("2006-01-02")
		} else if item.PublishedAt != nil {
			dateKey = item.PublishedAt.Format("2006-01-02")
		} else {
			continue
		}

		if _, exists := dateMap[dateKey]; !exists {
			dateOrder = append(dateOrder, dateKey)
		}
		dateMap[dateKey] = append(dateMap[dateKey], item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating calendar posts: %w", err)
	}

	entries := make([]model.CalendarEntry, 0, len(dateOrder))
	for _, date := range dateOrder {
		entries = append(entries, model.CalendarEntry{
			Date:  date,
			Posts: dateMap[date],
		})
	}

	return entries, nil
}

// SchedulePost sets a post to be published at a future time.
// The post must belong to the user and be in a schedulable status
// (draft, generated, or approved).
func (s *CalendarService) SchedulePost(ctx context.Context, userID, postID uuid.UUID, scheduledAt time.Time, timezone string) error {
	if scheduledAt.Before(time.Now().UTC()) {
		return fmt.Errorf("scheduled_at must be in the future")
	}

	if timezone == "" {
		timezone = "UTC"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var currentStatus string
	var authorID uuid.UUID
	err = tx.QueryRow(ctx,
		"SELECT status, author_id FROM posts WHERE id = $1 FOR UPDATE",
		postID,
	).Scan(&currentStatus, &authorID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("post not found")
		}
		return fmt.Errorf("query post: %w", err)
	}

	if authorID != userID {
		return fmt.Errorf("post not found")
	}

	if !schedulableStatuses[currentStatus] {
		return fmt.Errorf("post in status '%s' cannot be scheduled", currentStatus)
	}

	_, err = tx.Exec(ctx, `
		UPDATE posts
		SET status = 'scheduled'::post_status,
			scheduled_at = $2,
			timezone = $3,
			pre_schedule_status = $4,
			updated_at = NOW()
		WHERE id = $1`,
		postID, scheduledAt, timezone, currentStatus,
	)
	if err != nil {
		return fmt.Errorf("update post schedule: %w", err)
	}

	return tx.Commit(ctx)
}

// ReschedulePost changes the scheduled time of an already-scheduled post.
func (s *CalendarService) ReschedulePost(ctx context.Context, userID, postID uuid.UUID, scheduledAt time.Time, timezone string) error {
	if scheduledAt.Before(time.Now().UTC()) {
		return fmt.Errorf("scheduled_at must be in the future")
	}

	if timezone == "" {
		timezone = "UTC"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var currentStatus string
	var authorID uuid.UUID
	err = tx.QueryRow(ctx,
		"SELECT status, author_id FROM posts WHERE id = $1 FOR UPDATE",
		postID,
	).Scan(&currentStatus, &authorID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("post not found")
		}
		return fmt.Errorf("query post: %w", err)
	}

	if authorID != userID {
		return fmt.Errorf("post not found")
	}

	if currentStatus != "scheduled" {
		return fmt.Errorf("post is not scheduled (current status: '%s')", currentStatus)
	}

	_, err = tx.Exec(ctx, `
		UPDATE posts
		SET scheduled_at = $2,
			timezone = $3,
			updated_at = NOW()
		WHERE id = $1`,
		postID, scheduledAt, timezone,
	)
	if err != nil {
		return fmt.Errorf("update post schedule: %w", err)
	}

	return tx.Commit(ctx)
}

// UnschedulePost reverts a scheduled post back to its pre-schedule status
// and clears the scheduled_at time.
func (s *CalendarService) UnschedulePost(ctx context.Context, userID, postID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var currentStatus string
	var authorID uuid.UUID
	var preScheduleStatus *string
	err = tx.QueryRow(ctx,
		"SELECT status, author_id, pre_schedule_status FROM posts WHERE id = $1 FOR UPDATE",
		postID,
	).Scan(&currentStatus, &authorID, &preScheduleStatus)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("post not found")
		}
		return fmt.Errorf("query post: %w", err)
	}

	if authorID != userID {
		return fmt.Errorf("post not found")
	}

	if currentStatus != "scheduled" {
		return fmt.Errorf("post is not scheduled (current status: '%s')", currentStatus)
	}

	// Revert to the status the post had before scheduling; default to "draft"
	revertStatus := "draft"
	if preScheduleStatus != nil && *preScheduleStatus != "" {
		revertStatus = *preScheduleStatus
	}

	_, err = tx.Exec(ctx, `
		UPDATE posts
		SET status = $2::post_status,
			scheduled_at = NULL,
			timezone = 'UTC',
			pre_schedule_status = NULL,
			updated_at = NOW()
		WHERE id = $1`,
		postID, revertStatus,
	)
	if err != nil {
		return fmt.Errorf("unschedule post: %w", err)
	}

	return tx.Commit(ctx)
}

// BulkSchedule schedules multiple posts at once. Each item is processed
// independently -- partial failures are reported per-item.
func (s *CalendarService) BulkSchedule(ctx context.Context, userID uuid.UUID, items []model.BulkScheduleItem) ([]model.BulkScheduleResult, error) {
	results := make([]model.BulkScheduleResult, len(items))

	for i, item := range items {
		results[i].PostID = item.PostID

		err := s.SchedulePost(ctx, userID, item.PostID, item.ScheduledAt, item.Timezone)
		if err != nil {
			results[i].Success = false
			results[i].Error = err.Error()
		} else {
			results[i].Success = true
		}
	}

	return results, nil
}

// PickDueScheduled finds all posts where status = 'scheduled' and
// scheduled_at <= NOW(). Used by the schedule worker to transition
// posts into the publishing pipeline.
func (s *CalendarService) PickDueScheduled(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id FROM posts
		WHERE status = 'scheduled'
		  AND scheduled_at <= NOW()
		ORDER BY scheduled_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("query due scheduled posts: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan scheduled post id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating scheduled posts: %w", err)
	}

	return ids, nil
}

// TransitionToPublishing atomically releases a due scheduled post to the
// 'approved' status so the publish workers pick it up on their next tick
// (they move it to 'publishing'). Returns false if the post was already
// picked up by another worker (status changed). This uses SELECT FOR UPDATE
// SKIP LOCKED to avoid double-processing.
func (s *CalendarService) TransitionToPublishing(ctx context.Context, postID uuid.UUID) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var id uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT id FROM posts
		WHERE id = $1 AND status = 'scheduled'
		FOR UPDATE SKIP LOCKED`,
		postID,
	).Scan(&id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil // already picked up or status changed
		}
		return false, fmt.Errorf("lock post: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE posts
		SET status = 'approved'::post_status,
			pre_schedule_status = NULL,
			processing_started_at = NULL,
			updated_at = NOW()
		WHERE id = $1`,
		postID,
	)
	if err != nil {
		return false, fmt.Errorf("transition to publishing: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}

	return true, nil
}
