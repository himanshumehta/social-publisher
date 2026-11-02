package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type MobileService struct {
	pool *pgxpool.Pool
}

func NewMobileService(pool *pgxpool.Pool) *MobileService {
	return &MobileService{pool: pool}
}

// ---------------------------------------------------------------------------
// Push token management
// ---------------------------------------------------------------------------

// RegisterToken upserts a push token for a user+device pair.
func (s *MobileService) RegisterToken(ctx context.Context, userID uuid.UUID, req *model.RegisterTokenRequest) error {
	if req.DeviceID == "" {
		return fmt.Errorf("device_id is required")
	}
	if req.Token == "" {
		return fmt.Errorf("token is required")
	}
	if !validatePlatform(req.Platform) {
		return fmt.Errorf("invalid platform: must be ios, android, or web")
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO push_tokens (user_id, device_id, platform, token, is_active, last_used_at)
		VALUES ($1, $2, $3, $4, TRUE, NOW())
		ON CONFLICT (user_id, device_id)
		DO UPDATE SET token = $4, platform = $3, is_active = TRUE, last_used_at = NOW()
	`, userID, req.DeviceID, req.Platform, req.Token)
	if err != nil {
		return fmt.Errorf("register push token: %w", err)
	}
	return nil
}

// UnregisterToken deactivates a push token for a device.
func (s *MobileService) UnregisterToken(ctx context.Context, userID uuid.UUID, deviceID string) error {
	if deviceID == "" {
		return fmt.Errorf("device_id is required")
	}

	result, err := s.pool.Exec(ctx, `
		UPDATE push_tokens SET is_active = FALSE
		WHERE user_id = $1 AND device_id = $2
	`, userID, deviceID)
	if err != nil {
		return fmt.Errorf("unregister push token: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("token not found for device")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Push notification queue
// ---------------------------------------------------------------------------

// QueuePush inserts a push notification into the delivery queue.
func (s *MobileService) QueuePush(ctx context.Context, userID uuid.UUID, title, body string, data map[string]interface{}, priority string) error {
	if title == "" {
		return fmt.Errorf("title is required")
	}
	if priority != "normal" && priority != "high" {
		priority = "normal"
	}

	dataBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal push data: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO push_queue (user_id, title, body, data, priority)
		VALUES ($1, $2, $3, $4, $5)
	`, userID, title, body, dataBytes, priority)
	if err != nil {
		return fmt.Errorf("queue push notification: %w", err)
	}
	return nil
}

// ProcessPushQueue picks pending push notifications, groups by user, and
// delivers them. Returns the count of notifications processed.
// NOTE: Actual FCM/APNs delivery is a TODO. Currently marks as 'sent'.
func (s *MobileService) ProcessPushQueue(ctx context.Context) (int, error) {
	// Pick a batch of pending notifications (up to 100 per tick)
	rows, err := s.pool.Query(ctx, `
		UPDATE push_queue
		SET status = 'sent', sent_at = NOW()
		WHERE id IN (
			SELECT id FROM push_queue
			WHERE status = 'pending'
			ORDER BY created_at ASC
			LIMIT 100
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, user_id, title, body, data, priority
	`)
	if err != nil {
		return 0, fmt.Errorf("process push queue: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id, userID uuid.UUID
		var title, body, priority string
		var data []byte
		if err := rows.Scan(&id, &userID, &title, &body, &data, &priority); err != nil {
			return count, fmt.Errorf("scan push queue item: %w", err)
		}

		// TODO: Group by user_id, look up active push_tokens, and send via:
		//   - FCM (Firebase Cloud Messaging) for android
		//   - APNs (Apple Push Notification service) for ios
		//   - Web Push API for web
		// For now we mark as sent above. When real integration is added,
		// failures should UPDATE status='failed', error_message=$1.

		count++
	}
	if err := rows.Err(); err != nil {
		return count, fmt.Errorf("iterate push queue: %w", err)
	}

	return count, nil
}

// ---------------------------------------------------------------------------
// Sync queue
// ---------------------------------------------------------------------------

// QueueSync inserts an entity change into the sync queue for a user.
func (s *MobileService) QueueSync(ctx context.Context, userID uuid.UUID, entityType string, entityID uuid.UUID, action string, payload interface{}) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal sync payload: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO sync_queue (user_id, entity_type, entity_id, action, payload)
		VALUES ($1, $2, $3, $4, $5)
	`, userID, entityType, entityID, action, payloadBytes)
	if err != nil {
		return fmt.Errorf("queue sync entry: %w", err)
	}
	return nil
}

// GetSyncEntries returns sync entries after lastSequence using cursor-based pagination.
func (s *MobileService) GetSyncEntries(ctx context.Context, userID uuid.UUID, lastSequence int64, limit int) (*model.SyncResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	// Fetch limit+1 to detect HasMore
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, entity_type, entity_id, action, payload, sequence_num, created_at
		FROM sync_queue
		WHERE user_id = $1 AND sequence_num > $2
		ORDER BY sequence_num ASC
		LIMIT $3
	`, userID, lastSequence, limit+1)
	if err != nil {
		return nil, fmt.Errorf("get sync entries: %w", err)
	}
	defer rows.Close()

	var entries []model.SyncEntry
	for rows.Next() {
		var e model.SyncEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.EntityType, &e.EntityID, &e.Action, &e.Payload, &e.SequenceNum, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan sync entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sync entries: %w", err)
	}

	hasMore := len(entries) > limit
	if hasMore {
		entries = entries[:limit]
	}

	var lastSeq int64
	if len(entries) > 0 {
		lastSeq = entries[len(entries)-1].SequenceNum
	} else {
		lastSeq = lastSequence
	}

	if entries == nil {
		entries = []model.SyncEntry{}
	}

	return &model.SyncResponse{
		Entries:      entries,
		LastSequence: lastSeq,
		HasMore:      hasMore,
	}, nil
}

// ---------------------------------------------------------------------------
// Mobile notification feed
// ---------------------------------------------------------------------------

// GetNotificationFeed returns notifications in a mobile-friendly format.
func (s *MobileService) GetNotificationFeed(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.NotificationFeedItem, int, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	var total int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1`, userID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count notifications: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, title, COALESCE(body, ''), COALESCE(type, ''),
		       CASE WHEN is_read THEN created_at ELSE NULL END,
		       created_at
		FROM notifications
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query notification feed: %w", err)
	}
	defer rows.Close()

	var items []model.NotificationFeedItem
	for rows.Next() {
		var item model.NotificationFeedItem
		if err := rows.Scan(&item.ID, &item.Title, &item.Body, &item.Type, &item.ReadAt, &item.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan notification feed: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate notification feed: %w", err)
	}
	if items == nil {
		items = []model.NotificationFeedItem{}
	}

	return items, total, nil
}

// ---------------------------------------------------------------------------
// Quick post creation
// ---------------------------------------------------------------------------

// QuickCreatePost creates a post with minimal fields for mobile clients.
// Delegates to the posts table directly, matching CreatePostRequest semantics.
func (s *MobileService) QuickCreatePost(ctx context.Context, userID uuid.UUID, req *model.QuickPostRequest) (*uuid.UUID, error) {
	if req.Title == "" {
		return nil, fmt.Errorf("title is required")
	}

	platforms := req.TargetPlatforms
	if len(platforms) == 0 {
		platforms = []string{"twitter"}
	}

	var postID uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO posts (author_id, title, content, target_platforms, auto_publish, status, slug)
		VALUES ($1, $2, $3, $4, $5, 'draft', $6)
		RETURNING id
	`, userID, req.Title, req.Content, platforms, req.AutoPublish,
		"mobile-"+uuid.New().String()[:8],
	).Scan(&postID)
	if err != nil {
		return nil, fmt.Errorf("quick create post: %w", err)
	}

	return &postID, nil
}

// ---------------------------------------------------------------------------
// Mobile dashboard
// ---------------------------------------------------------------------------

// GetMobileDashboard returns aggregated counts and recent posts for mobile home screens.
func (s *MobileService) GetMobileDashboard(ctx context.Context, userID uuid.UUID) (*model.MobileDashboard, error) {
	dash := &model.MobileDashboard{}

	err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'draft'),
			COUNT(*) FILTER (WHERE status = 'scheduled'),
			COUNT(*) FILTER (WHERE status = 'published'),
			(SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = FALSE)
		FROM posts
		WHERE author_id = $1
	`, userID).Scan(
		&dash.PostCount,
		&dash.DraftCount,
		&dash.ScheduledCount,
		&dash.PublishedCount,
		&dash.UnreadNotifications,
	)
	if err != nil {
		return nil, fmt.Errorf("get mobile dashboard counts: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, title, status,
		       COALESCE(target_platforms[1], ''),
		       created_at
		FROM posts
		WHERE author_id = $1
		ORDER BY created_at DESC
		LIMIT 10
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get mobile recent posts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item model.MobilePostItem
		if err := rows.Scan(&item.ID, &item.Title, &item.Status, &item.Platform, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan mobile post: %w", err)
		}
		dash.RecentPosts = append(dash.RecentPosts, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mobile posts: %w", err)
	}
	if dash.RecentPosts == nil {
		dash.RecentPosts = []model.MobilePostItem{}
	}

	return dash, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// validatePlatform checks if a push token platform is valid.
func validatePlatform(platform string) bool {
	switch platform {
	case "ios", "android", "web":
		return true
	default:
		return false
	}
}

// buildPushPayload constructs an FCM-compatible JSON payload.
func buildPushPayload(title, body string, data map[string]interface{}) ([]byte, error) {
	payload := map[string]interface{}{
		"notification": map[string]string{
			"title": title,
			"body":  body,
		},
	}
	if len(data) > 0 {
		payload["data"] = data
	}
	return json.Marshal(payload)
}
