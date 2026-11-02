package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/model"
)

func TestNewsletterService_Create(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewNewsletterService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	nl, err := svc.Create(ctx, userID, &model.CreateNewsletterRequest{
		Name:        "Bike Updates Weekly",
		Description: "Weekly newsletter about bikes and cars",
		FromName:    "Acme",
		FromEmail:   "newsletter@example.com",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if nl.ID == uuid.Nil {
		t.Error("expected non-nil newsletter ID")
	}
	if nl.Name != "Bike Updates Weekly" {
		t.Errorf("expected name 'Bike Updates Weekly', got %s", nl.Name)
	}
	if !nl.IsActive {
		t.Error("expected newsletter to be active by default")
	}
	if nl.ColorPrimary != "#2563EB" {
		t.Errorf("expected default primary color, got %s", nl.ColorPrimary)
	}
}

func TestNewsletterService_Get(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewNewsletterService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	created, _ := svc.Create(ctx, userID, &model.CreateNewsletterRequest{Name: "Get Test"})

	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, got.ID)
	}
}

func TestNewsletterService_ListByUser(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewNewsletterService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	for i := 0; i < 3; i++ {
		_, _ = svc.Create(ctx, userID, &model.CreateNewsletterRequest{
			Name: "NL " + uuid.New().String()[:8],
		})
	}

	newsletters, err := svc.ListByUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListByUser() error = %v", err)
	}
	if len(newsletters) < 3 {
		t.Errorf("expected at least 3 newsletters, got %d", len(newsletters))
	}
}

func TestNewsletterService_Update(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewNewsletterService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	nl, _ := svc.Create(ctx, userID, &model.CreateNewsletterRequest{Name: "Update Test"})

	newName := "Updated Newsletter"
	updated, err := svc.Update(ctx, userID, nl.ID, &model.UpdateNewsletterRequest{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != "Updated Newsletter" {
		t.Errorf("expected updated name, got %s", updated.Name)
	}
}

func TestNewsletterService_Delete(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewNewsletterService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	nl, _ := svc.Create(ctx, userID, &model.CreateNewsletterRequest{Name: "Delete Test"})

	if err := svc.Delete(ctx, userID, nl.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err := svc.Get(ctx, nl.ID)
	if err == nil {
		t.Error("expected error getting deleted newsletter")
	}
}

// ── Subscriber Tests ──

func TestNewsletterService_AddSubscriber(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewNewsletterService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")
	nl, _ := svc.Create(ctx, userID, &model.CreateNewsletterRequest{Name: "Sub Test"})

	sub, err := svc.AddSubscriber(ctx, nl.ID, &model.AddSubscriberRequest{
		Email:  "viewer@example.com",
		Name:   "Test Viewer",
		Source: "widget",
	})
	if err != nil {
		t.Fatalf("AddSubscriber() error = %v", err)
	}
	if sub.Email != "viewer@example.com" {
		t.Errorf("expected email viewer@example.com, got %s", sub.Email)
	}
	if sub.Status != "active" {
		t.Errorf("expected status active, got %s", sub.Status)
	}

	// Check subscriber count updated
	updated, _ := svc.Get(ctx, nl.ID)
	if updated.SubscriberCount != 1 {
		t.Errorf("expected subscriber count 1, got %d", updated.SubscriberCount)
	}
}

func TestNewsletterService_BulkAddSubscribers(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewNewsletterService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")
	nl, _ := svc.Create(ctx, userID, &model.CreateNewsletterRequest{Name: "Bulk Sub Test"})

	count, err := svc.BulkAddSubscribers(ctx, nl.ID, &model.BulkAddSubscribersRequest{
		Subscribers: []model.AddSubscriberRequest{
			{Email: "bulk1@example.com", Name: "Bulk 1"},
			{Email: "bulk2@example.com", Name: "Bulk 2"},
			{Email: "bulk3@example.com", Name: "Bulk 3"},
		},
	})
	if err != nil {
		t.Fatalf("BulkAddSubscribers() error = %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 added, got %d", count)
	}
}

func TestNewsletterService_Unsubscribe(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewNewsletterService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")
	nl, _ := svc.Create(ctx, userID, &model.CreateNewsletterRequest{Name: "Unsub Test"})

	email := "unsub-" + uuid.New().String()[:8] + "@example.com"
	_, _ = svc.AddSubscriber(ctx, nl.ID, &model.AddSubscriberRequest{Email: email})

	if err := svc.Unsubscribe(ctx, nl.ID, email); err != nil {
		t.Fatalf("Unsubscribe() error = %v", err)
	}

	// Verify count decreased
	updated, _ := svc.Get(ctx, nl.ID)
	if updated.SubscriberCount != 0 {
		t.Errorf("expected subscriber count 0 after unsubscribe, got %d", updated.SubscriberCount)
	}
}

func TestNewsletterService_ListSubscribers(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewNewsletterService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")
	nl, _ := svc.Create(ctx, userID, &model.CreateNewsletterRequest{Name: "List Sub Test"})

	for i := 0; i < 3; i++ {
		email := "list-" + uuid.New().String()[:8] + "@example.com"
		_, _ = svc.AddSubscriber(ctx, nl.ID, &model.AddSubscriberRequest{Email: email})
	}

	subs, err := svc.ListSubscribers(ctx, nl.ID, "")
	if err != nil {
		t.Fatalf("ListSubscribers() error = %v", err)
	}
	if len(subs) < 3 {
		t.Errorf("expected at least 3 subscribers, got %d", len(subs))
	}

	// Filter by status
	active, err := svc.ListSubscribers(ctx, nl.ID, "active")
	if err != nil {
		t.Fatalf("ListSubscribers(active) error = %v", err)
	}
	if len(active) < 3 {
		t.Errorf("expected at least 3 active subscribers, got %d", len(active))
	}
}

// ── Issue Tests ──

func TestNewsletterService_CreateIssue(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewNewsletterService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")
	nl, _ := svc.Create(ctx, userID, &model.CreateNewsletterRequest{Name: "Issue Test"})

	issue, err := svc.CreateIssue(ctx, nl.ID, &model.CreateIssueRequest{
		Subject:     "Weekly Update #1",
		PreviewText: "This week's top bike news",
		HTMLContent: "<h1>Weekly Update</h1><p>Content here</p>",
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if issue.Subject != "Weekly Update #1" {
		t.Errorf("expected subject 'Weekly Update #1', got %s", issue.Subject)
	}
	if issue.Status != "draft" {
		t.Errorf("expected status draft, got %s", issue.Status)
	}
}

func TestNewsletterService_SendIssue(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewNewsletterService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")
	nl, _ := svc.Create(ctx, userID, &model.CreateNewsletterRequest{Name: "Send Issue Test"})

	// Add subscribers first
	for i := 0; i < 3; i++ {
		email := "send-" + uuid.New().String()[:8] + "@example.com"
		_, _ = svc.AddSubscriber(ctx, nl.ID, &model.AddSubscriberRequest{Email: email})
	}

	issue, _ := svc.CreateIssue(ctx, nl.ID, &model.CreateIssueRequest{
		Subject:     "Send Test Issue",
		HTMLContent: "<p>Test content</p>",
	})

	if err := svc.SendIssue(ctx, userID, issue.ID); err != nil {
		t.Fatalf("SendIssue() error = %v", err)
	}

	sent, _ := svc.GetIssue(ctx, issue.ID)
	if sent.Status != "sent" {
		t.Errorf("expected status sent, got %s", sent.Status)
	}
	if sent.SentCount != 3 {
		t.Errorf("expected sent_count 3, got %d", sent.SentCount)
	}
	if sent.SentAt == nil {
		t.Error("expected sent_at to be set")
	}
}

func TestNewsletterService_DeleteIssue(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewNewsletterService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")
	nl, _ := svc.Create(ctx, userID, &model.CreateNewsletterRequest{Name: "Delete Issue Test"})

	issue, _ := svc.CreateIssue(ctx, nl.ID, &model.CreateIssueRequest{
		Subject:     "Delete Me",
		HTMLContent: "<p>Delete</p>",
	})

	if err := svc.DeleteIssue(ctx, nl.ID, issue.ID); err != nil {
		t.Fatalf("DeleteIssue() error = %v", err)
	}
}

// ── Stats Tests ──

func TestNewsletterService_GetStats(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewNewsletterService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")
	nl, _ := svc.Create(ctx, userID, &model.CreateNewsletterRequest{Name: "Stats Test"})

	// Add subscribers
	for i := 0; i < 5; i++ {
		email := "stats-" + uuid.New().String()[:8] + "@example.com"
		_, _ = svc.AddSubscriber(ctx, nl.ID, &model.AddSubscriberRequest{Email: email})
	}

	stats, err := svc.GetStats(ctx, nl.ID)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if stats.ActiveSubscribers != 5 {
		t.Errorf("expected 5 active subscribers, got %d", stats.ActiveSubscribers)
	}
	if stats.TotalSubscribers != 5 {
		t.Errorf("expected 5 total subscribers, got %d", stats.TotalSubscribers)
	}
}

func TestNewsletterService_ResubscribeOnConflict(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewNewsletterService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")
	nl, _ := svc.Create(ctx, userID, &model.CreateNewsletterRequest{Name: "Resub Test"})

	email := "resub-" + uuid.New().String()[:8] + "@example.com"
	_, _ = svc.AddSubscriber(ctx, nl.ID, &model.AddSubscriberRequest{Email: email})
	_ = svc.Unsubscribe(ctx, nl.ID, email)

	// Re-add should re-activate
	sub, err := svc.AddSubscriber(ctx, nl.ID, &model.AddSubscriberRequest{Email: email})
	if err != nil {
		t.Fatalf("re-subscribe error = %v", err)
	}
	if sub.Status != "active" {
		t.Errorf("expected re-subscribed status to be active, got %s", sub.Status)
	}
}
