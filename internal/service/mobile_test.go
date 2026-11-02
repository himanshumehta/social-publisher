package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ---------------------------------------------------------------------------
// validatePlatform
// ---------------------------------------------------------------------------

func TestValidatePlatform_Valid(t *testing.T) {
	for _, p := range []string{"ios", "android", "web"} {
		if !validatePlatform(p) {
			t.Errorf("expected validatePlatform(%q) = true", p)
		}
	}
}

func TestValidatePlatform_Invalid(t *testing.T) {
	for _, p := range []string{"", "windows", "macos", "iOS", "ANDROID"} {
		if validatePlatform(p) {
			t.Errorf("expected validatePlatform(%q) = false", p)
		}
	}
}

// ---------------------------------------------------------------------------
// buildPushPayload
// ---------------------------------------------------------------------------

func TestBuildPushPayload_WithData(t *testing.T) {
	data := map[string]interface{}{
		"post_id": "abc-123",
		"action":  "open_post",
	}
	payload, err := buildPushPayload("New Post", "Your post was published", data)
	if err != nil {
		t.Fatalf("buildPushPayload failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}

	notif, ok := parsed["notification"].(map[string]interface{})
	if !ok {
		t.Fatal("expected notification key in payload")
	}
	if notif["title"] != "New Post" {
		t.Errorf("expected title='New Post', got %v", notif["title"])
	}
	if notif["body"] != "Your post was published" {
		t.Errorf("expected body='Your post was published', got %v", notif["body"])
	}

	d, ok := parsed["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected data key in payload")
	}
	if d["post_id"] != "abc-123" {
		t.Errorf("expected data.post_id='abc-123', got %v", d["post_id"])
	}
}

func TestBuildPushPayload_EmptyData(t *testing.T) {
	payload, err := buildPushPayload("Title", "Body", nil)
	if err != nil {
		t.Fatalf("buildPushPayload failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}

	if _, ok := parsed["data"]; ok {
		t.Error("expected no data key in payload when data is nil")
	}

	notif, ok := parsed["notification"].(map[string]interface{})
	if !ok {
		t.Fatal("expected notification key")
	}
	if notif["title"] != "Title" {
		t.Errorf("expected title='Title', got %v", notif["title"])
	}
}

// ---------------------------------------------------------------------------
// Database-backed tests (require test database)
// ---------------------------------------------------------------------------

func createMobileTestUser(t *testing.T, pool interface {
	Exec(context.Context, string, ...interface{}) (interface{ RowsAffected() int64 }, error)
}) (uuid.UUID, string) {
	t.Helper()
	// Use the test pool directly
	return uuid.UUID{}, ""
}

func TestRegisterToken_Upsert(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	email := fmt.Sprintf("test-mobile-reg-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Mobile Register Test", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM push_tokens WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewMobileService(pool)

	// First registration
	req := &model.RegisterTokenRequest{
		DeviceID: "device-001",
		Platform: "ios",
		Token:    "token-aaa",
	}
	if err := svc.RegisterToken(ctx, userID, req); err != nil {
		t.Fatalf("RegisterToken (first) failed: %v", err)
	}

	// Upsert with new token for same device
	req.Token = "token-bbb"
	if err := svc.RegisterToken(ctx, userID, req); err != nil {
		t.Fatalf("RegisterToken (upsert) failed: %v", err)
	}

	// Verify only one row exists with updated token
	var count int
	var token string
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*), MAX(token) FROM push_tokens WHERE user_id = $1 AND device_id = $2`,
		userID, "device-001",
	).Scan(&count, &token)
	if err != nil {
		t.Fatalf("verify upsert failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 push token row, got %d", count)
	}
	if token != "token-bbb" {
		t.Errorf("expected token='token-bbb', got %q", token)
	}
}

func TestGetSyncEntries_CursorPagination(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	email := fmt.Sprintf("test-mobile-sync-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Mobile Sync Test", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM sync_queue WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewMobileService(pool)

	// Insert 5 sync entries
	for i := 0; i < 5; i++ {
		err := svc.QueueSync(ctx, userID, "post", uuid.New(), "create", map[string]string{"index": fmt.Sprintf("%d", i)})
		if err != nil {
			t.Fatalf("QueueSync failed: %v", err)
		}
	}

	// Fetch first page (limit 3)
	resp, err := svc.GetSyncEntries(ctx, userID, 0, 3)
	if err != nil {
		t.Fatalf("GetSyncEntries page 1 failed: %v", err)
	}
	if len(resp.Entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(resp.Entries))
	}
	if !resp.HasMore {
		t.Error("expected HasMore=true for first page")
	}

	// Fetch second page using cursor
	resp2, err := svc.GetSyncEntries(ctx, userID, resp.LastSequence, 3)
	if err != nil {
		t.Fatalf("GetSyncEntries page 2 failed: %v", err)
	}
	if len(resp2.Entries) != 2 {
		t.Errorf("expected 2 entries on second page, got %d", len(resp2.Entries))
	}
	if resp2.HasMore {
		t.Error("expected HasMore=false for last page")
	}
}

func TestGetSyncEntries_NoNewEntries(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	email := fmt.Sprintf("test-mobile-syncempty-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Mobile Sync Empty Test", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewMobileService(pool)

	resp, err := svc.GetSyncEntries(ctx, userID, 0, 50)
	if err != nil {
		t.Fatalf("GetSyncEntries failed: %v", err)
	}
	if len(resp.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(resp.Entries))
	}
	if resp.HasMore {
		t.Error("expected HasMore=false when no entries")
	}
	if resp.LastSequence != 0 {
		t.Errorf("expected LastSequence=0, got %d", resp.LastSequence)
	}
}

func TestGetSyncEntries_HasMore(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	email := fmt.Sprintf("test-mobile-syncmore-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Mobile Sync HasMore Test", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM sync_queue WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewMobileService(pool)

	// Insert exactly limit+1 entries
	for i := 0; i < 4; i++ {
		if err := svc.QueueSync(ctx, userID, "post", uuid.New(), "create", nil); err != nil {
			t.Fatalf("QueueSync failed: %v", err)
		}
	}

	resp, err := svc.GetSyncEntries(ctx, userID, 0, 3)
	if err != nil {
		t.Fatalf("GetSyncEntries failed: %v", err)
	}
	if len(resp.Entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(resp.Entries))
	}
	if !resp.HasMore {
		t.Error("expected HasMore=true when there are more entries")
	}
}

func TestQuickCreatePost_MinimalFields(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	email := fmt.Sprintf("test-mobile-quick-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Mobile Quick Post Test", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewMobileService(pool)

	req := &model.QuickPostRequest{
		Title: "Quick mobile post",
	}
	postID, err := svc.QuickCreatePost(ctx, userID, req)
	if err != nil {
		t.Fatalf("QuickCreatePost failed: %v", err)
	}
	if postID == nil {
		t.Fatal("expected non-nil post ID")
	}

	// Verify the post exists with correct defaults
	var status string
	var autoPublish bool
	err = pool.QueryRow(ctx,
		`SELECT status, auto_publish FROM posts WHERE id = $1`, *postID,
	).Scan(&status, &autoPublish)
	if err != nil {
		t.Fatalf("verify post failed: %v", err)
	}
	if status != "draft" {
		t.Errorf("expected status='draft', got %q", status)
	}
	if autoPublish != false {
		t.Error("expected auto_publish=false for minimal request")
	}
}

func TestGetMobileDashboard_Counts(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	email := fmt.Sprintf("test-mobile-dash-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Mobile Dashboard Test", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewMobileService(pool)

	// Create a draft post via quick-create
	_, err = svc.QuickCreatePost(ctx, userID, &model.QuickPostRequest{
		Title: "Dashboard test post",
	})
	if err != nil {
		t.Fatalf("QuickCreatePost failed: %v", err)
	}

	dash, err := svc.GetMobileDashboard(ctx, userID)
	if err != nil {
		t.Fatalf("GetMobileDashboard failed: %v", err)
	}

	if dash.PostCount < 1 {
		t.Errorf("expected PostCount >= 1, got %d", dash.PostCount)
	}
	if dash.DraftCount < 1 {
		t.Errorf("expected DraftCount >= 1, got %d", dash.DraftCount)
	}
	if dash.UnreadNotifications < 0 {
		t.Errorf("expected UnreadNotifications >= 0, got %d", dash.UnreadNotifications)
	}
	if dash.RecentPosts == nil {
		t.Error("expected RecentPosts to be non-nil slice")
	}
	if len(dash.RecentPosts) < 1 {
		t.Errorf("expected at least 1 recent post, got %d", len(dash.RecentPosts))
	}
}

func TestProcessPushQueue_GroupsByUser(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	email := fmt.Sprintf("test-mobile-push-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Mobile Push Test", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM push_queue WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewMobileService(pool)

	// Queue 3 notifications for the same user
	for i := 0; i < 3; i++ {
		err := svc.QueuePush(ctx, userID,
			fmt.Sprintf("Title %d", i),
			fmt.Sprintf("Body %d", i),
			map[string]interface{}{"index": i},
			"normal",
		)
		if err != nil {
			t.Fatalf("QueuePush failed: %v", err)
		}
	}

	// Process the queue
	processed, err := svc.ProcessPushQueue(ctx)
	if err != nil {
		t.Fatalf("ProcessPushQueue failed: %v", err)
	}
	if processed != 3 {
		t.Errorf("expected 3 processed, got %d", processed)
	}

	// Verify all are marked as sent
	var pendingCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM push_queue WHERE user_id = $1 AND status = 'pending'`, userID,
	).Scan(&pendingCount)
	if err != nil {
		t.Fatalf("verify pending count failed: %v", err)
	}
	if pendingCount != 0 {
		t.Errorf("expected 0 pending, got %d", pendingCount)
	}

	var sentCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM push_queue WHERE user_id = $1 AND status = 'sent'`, userID,
	).Scan(&sentCount)
	if err != nil {
		t.Fatalf("verify sent count failed: %v", err)
	}
	if sentCount != 3 {
		t.Errorf("expected 3 sent, got %d", sentCount)
	}
}
