package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/himanshumehta/social-publisher/internal/model"
)

func TestNotificationService_CRUD(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	// Get a real user ID
	var userID uuid.UUID
	err := pool.QueryRow(ctx, "SELECT id FROM users LIMIT 1").Scan(&userID)
	if err != nil {
		t.Skipf("skipping: no users in test database: %v", err)
	}

	svc := NewNotificationService(pool)

	// Create
	notif, err := svc.Create(ctx, userID, "Test Title", "Test body", "/test-link", model.NotifSystem)
	if err != nil {
		t.Fatalf("Create notification failed: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM notifications WHERE id = $1", notif.ID)
	}()

	if notif.ID == uuid.Nil {
		t.Fatal("expected notification to have an ID")
	}
	if notif.Title != "Test Title" {
		t.Errorf("expected title 'Test Title', got %q", notif.Title)
	}
	if notif.IsRead {
		t.Error("expected notification to be unread by default")
	}

	// UnreadCount
	count, err := svc.UnreadCount(ctx, userID)
	if err != nil {
		t.Fatalf("UnreadCount failed: %v", err)
	}
	if count < 1 {
		t.Error("expected at least 1 unread notification")
	}

	// List
	list, total, err := svc.ListByUser(ctx, userID, false, 10, 0)
	if err != nil {
		t.Fatalf("ListByUser failed: %v", err)
	}
	if total < 1 {
		t.Error("expected at least 1 notification in total")
	}
	found := false
	for _, n := range list {
		if n.ID == notif.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected created notification in list")
	}

	// List unread only
	unreadList, _, err := svc.ListByUser(ctx, userID, true, 10, 0)
	if err != nil {
		t.Fatalf("ListByUser (unread) failed: %v", err)
	}
	for _, n := range unreadList {
		if n.IsRead {
			t.Error("expected only unread notifications in unread filter")
		}
	}

	// MarkAsRead
	err = svc.MarkAsRead(ctx, userID, notif.ID)
	if err != nil {
		t.Fatalf("MarkAsRead failed: %v", err)
	}

	// Verify it's now read
	list2, _, err := svc.ListByUser(ctx, userID, false, 100, 0)
	if err != nil {
		t.Fatalf("ListByUser after mark read failed: %v", err)
	}
	for _, n := range list2 {
		if n.ID == notif.ID && !n.IsRead {
			t.Error("expected notification to be marked as read")
		}
	}

	// Delete
	err = svc.Delete(ctx, userID, notif.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}

func TestNotificationService_MarkAsRead_WrongUser(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	var userID uuid.UUID
	err := pool.QueryRow(ctx, "SELECT id FROM users LIMIT 1").Scan(&userID)
	if err != nil {
		t.Skipf("skipping: no users in test database: %v", err)
	}

	svc := NewNotificationService(pool)
	notif, err := svc.Create(ctx, userID, "Ownership Test", "", "", model.NotifSystem)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM notifications WHERE id = $1", notif.ID)
	}()

	// Try to mark as read with a different user
	fakeUser := uuid.New()
	err = svc.MarkAsRead(ctx, fakeUser, notif.ID)
	if err == nil {
		t.Error("expected error when marking notification of a different user as read")
	}
}

func TestNotificationService_MarkAllAsRead(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	var userID uuid.UUID
	err := pool.QueryRow(ctx, "SELECT id FROM users LIMIT 1").Scan(&userID)
	if err != nil {
		t.Skipf("skipping: no users in test database: %v", err)
	}

	svc := NewNotificationService(pool)

	// Create multiple
	n1, _ := svc.Create(ctx, userID, "Notif 1", "", "", model.NotifSystem)
	n2, _ := svc.Create(ctx, userID, "Notif 2", "", "", model.NotifSystem)
	defer func() {
		if n1 != nil {
			_, _ = pool.Exec(ctx, "DELETE FROM notifications WHERE id = $1", n1.ID)
		}
		if n2 != nil {
			_, _ = pool.Exec(ctx, "DELETE FROM notifications WHERE id = $1", n2.ID)
		}
	}()

	updated, err := svc.MarkAllAsRead(ctx, userID)
	if err != nil {
		t.Fatalf("MarkAllAsRead failed: %v", err)
	}
	if updated < 2 {
		t.Errorf("expected at least 2 updated, got %d", updated)
	}

	count, _ := svc.UnreadCount(ctx, userID)
	if count != 0 {
		t.Errorf("expected 0 unread after mark all, got %d", count)
	}
}

func TestNotificationService_DeleteNotFound(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	svc := NewNotificationService(pool)
	err := svc.Delete(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Error("expected error deleting non-existent notification")
	}
}

func TestNotificationService_Pagination(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	var userID uuid.UUID
	err := pool.QueryRow(ctx, "SELECT id FROM users LIMIT 1").Scan(&userID)
	if err != nil {
		t.Skipf("skipping: no users in test database: %v", err)
	}

	svc := NewNotificationService(pool)

	// Create 5 notifications
	var ids []uuid.UUID
	for i := 0; i < 5; i++ {
		n, err := svc.Create(ctx, userID, "Page Test", "", "", model.NotifSystem)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		ids = append(ids, n.ID)
	}
	defer func() {
		for _, id := range ids {
			_, _ = pool.Exec(ctx, "DELETE FROM notifications WHERE id = $1", id)
		}
	}()

	// Fetch page 1 (limit=2, offset=0)
	page1, total, err := svc.ListByUser(ctx, userID, false, 2, 0)
	if err != nil {
		t.Fatalf("ListByUser page 1 failed: %v", err)
	}
	if len(page1) != 2 {
		t.Errorf("expected 2 results on page 1, got %d", len(page1))
	}
	if total < 5 {
		t.Errorf("expected total >= 5, got %d", total)
	}

	// Fetch page 2
	page2, _, err := svc.ListByUser(ctx, userID, false, 2, 2)
	if err != nil {
		t.Fatalf("ListByUser page 2 failed: %v", err)
	}
	if len(page2) != 2 {
		t.Errorf("expected 2 results on page 2, got %d", len(page2))
	}

	// Ensure pages don't overlap
	if len(page1) > 0 && len(page2) > 0 && page1[0].ID == page2[0].ID {
		t.Error("page 1 and page 2 should have different notifications")
	}
}
