package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type NotificationService struct {
	pool *pgxpool.Pool
}

func NewNotificationService(pool *pgxpool.Pool) *NotificationService {
	return &NotificationService{pool: pool}
}

// ListByUser returns paginated notifications for a user, ordered newest first.
func (s *NotificationService) ListByUser(ctx context.Context, userID uuid.UUID, unreadOnly bool, limit, offset int) ([]model.Notification, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var args []interface{}
	where := "WHERE user_id = $1"
	args = append(args, userID)
	argIdx := 2

	if unreadOnly {
		where += " AND is_read = FALSE"
	}

	var total int
	err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM notifications "+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count notifications: %w", err)
	}

	query := fmt.Sprintf(
		`SELECT id, user_id, title, COALESCE(body, ''), COALESCE(link, ''), is_read, COALESCE(type, ''), created_at
		 FROM notifications %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query notifications: %w", err)
	}
	defer rows.Close()

	var notifications []model.Notification
	for rows.Next() {
		var n model.Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Title, &n.Body, &n.Link, &n.IsRead, &n.Type, &n.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan notification: %w", err)
		}
		notifications = append(notifications, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate notifications: %w", err)
	}
	if notifications == nil {
		notifications = []model.Notification{}
	}
	return notifications, total, nil
}

// UnreadCount returns the count of unread notifications for a user.
func (s *NotificationService) UnreadCount(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = FALSE`, userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unread notifications: %w", err)
	}
	return count, nil
}

// MarkAsRead marks a single notification as read. Verifies ownership.
func (s *NotificationService) MarkAsRead(ctx context.Context, userID, notificationID uuid.UUID) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE notifications SET is_read = TRUE WHERE id = $1 AND user_id = $2`,
		notificationID, userID)
	if err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("notification not found")
	}
	return nil
}

// MarkAllAsRead marks all unread notifications as read for a user.
func (s *NotificationService) MarkAllAsRead(ctx context.Context, userID uuid.UUID) (int64, error) {
	result, err := s.pool.Exec(ctx,
		`UPDATE notifications SET is_read = TRUE WHERE user_id = $1 AND is_read = FALSE`,
		userID)
	if err != nil {
		return 0, fmt.Errorf("mark all read: %w", err)
	}
	return result.RowsAffected(), nil
}

// Create inserts a new notification. Used internally by other services.
func (s *NotificationService) Create(ctx context.Context, userID uuid.UUID, title, body, link string, notifType model.NotificationType) (*model.Notification, error) {
	var n model.Notification
	err := s.pool.QueryRow(ctx,
		`INSERT INTO notifications (user_id, title, body, link, type, is_read)
		 VALUES ($1, $2, $3, $4, $5, FALSE)
		 RETURNING id, user_id, title, COALESCE(body, ''), COALESCE(link, ''), is_read, COALESCE(type, ''), created_at`,
		userID, title, nilIfEmptyStr(body), nilIfEmptyStr(link), string(notifType),
	).Scan(&n.ID, &n.UserID, &n.Title, &n.Body, &n.Link, &n.IsRead, &n.Type, &n.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert notification: %w", err)
	}
	return &n, nil
}

// CreateTx inserts a notification within an existing transaction.
func (s *NotificationService) CreateTx(ctx context.Context, tx pgx.Tx, userID uuid.UUID, title, body, link string, notifType model.NotificationType) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO notifications (user_id, title, body, link, type, is_read)
		 VALUES ($1, $2, $3, $4, $5, FALSE)`,
		userID, title, nilIfEmptyStr(body), nilIfEmptyStr(link), string(notifType))
	if err != nil {
		return fmt.Errorf("insert notification: %w", err)
	}
	return nil
}

// Delete removes a notification. Verifies ownership.
func (s *NotificationService) Delete(ctx context.Context, userID, notificationID uuid.UUID) error {
	result, err := s.pool.Exec(ctx,
		`DELETE FROM notifications WHERE id = $1 AND user_id = $2`,
		notificationID, userID)
	if err != nil {
		return fmt.Errorf("delete notification: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("notification not found")
	}
	return nil
}
