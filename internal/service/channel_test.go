package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

func TestChannelService_CreateAndGet(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	// Create an isolated test category to avoid unique constraint violations
	testCatID := uuid.New()
	testCatName := fmt.Sprintf("test-channel-cat-%s", testCatID.String()[:8])
	_, err := pool.Exec(ctx,
		"INSERT INTO categories (id, name, slug) VALUES ($1, $2, $3)",
		testCatID, testCatName, testCatName)
	if err != nil {
		t.Fatalf("failed to create test category: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM channels WHERE category_id = $1", testCatID)
		_, _ = pool.Exec(ctx, "DELETE FROM categories WHERE id = $1", testCatID)
	}()

	svc := NewChannelService(pool)

	ch, err := svc.Create(ctx, CreateChannelRequest{
		CategoryID:  &testCatID,
		Platform:    "youtube",
		ChannelName: "Test Channel Service",
		ChannelID:   "UC_test_123",
		ChannelURL:  "https://youtube.com/@testchannel",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if ch.ID == uuid.Nil {
		t.Fatal("expected channel to have an ID")
	}
	if ch.ChannelName != "Test Channel Service" {
		t.Errorf("expected channel_name 'Test Channel Service', got %q", ch.ChannelName)
	}
	if ch.Platform != "youtube" {
		t.Errorf("expected platform 'youtube', got %q", ch.Platform)
	}
	if !ch.IsActive {
		t.Error("expected new channel to be active by default")
	}

	// GetByID
	fetched, err := svc.GetByID(ctx, ch.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if fetched.ChannelName != ch.ChannelName {
		t.Errorf("GetByID: expected channel_name %q, got %q", ch.ChannelName, fetched.ChannelName)
	}
	if fetched.ChannelURL != "https://youtube.com/@testchannel" {
		t.Errorf("GetByID: expected channel_url, got %q", fetched.ChannelURL)
	}
}

func TestChannelService_Create_RequiresName(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	svc := NewChannelService(pool)
	catID := uuid.New()
	_, err := svc.Create(context.Background(), CreateChannelRequest{
		CategoryID: &catID,
		Platform:   "youtube",
	})
	if err == nil {
		t.Error("expected error when channel_name is empty")
	}
}

func TestChannelService_Create_RequiresPlatform(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	svc := NewChannelService(pool)
	catID := uuid.New()
	_, err := svc.Create(context.Background(), CreateChannelRequest{
		CategoryID:  &catID,
		ChannelName: "Some Channel",
	})
	if err == nil {
		t.Error("expected error when platform is empty")
	}
}

func TestChannelService_Update(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	testCatID := uuid.New()
	testCatName := fmt.Sprintf("test-update-cat-%s", testCatID.String()[:8])
	_, err := pool.Exec(ctx,
		"INSERT INTO categories (id, name, slug) VALUES ($1, $2, $3)",
		testCatID, testCatName, testCatName)
	if err != nil {
		t.Fatalf("failed to create test category: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM channels WHERE category_id = $1", testCatID)
		_, _ = pool.Exec(ctx, "DELETE FROM categories WHERE id = $1", testCatID)
	}()

	svc := NewChannelService(pool)
	ch, err := svc.Create(ctx, CreateChannelRequest{
		CategoryID:  &testCatID,
		Platform:    "instagram",
		ChannelName: "Before Update",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	newName := "After Update"
	newURL := "https://instagram.com/updated"
	updated, err := svc.Update(ctx, ch.ID, UpdateChannelRequest{
		ChannelName: &newName,
		ChannelURL:  &newURL,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.ChannelName != "After Update" {
		t.Errorf("expected channel_name 'After Update', got %q", updated.ChannelName)
	}
	if updated.ChannelURL != "https://instagram.com/updated" {
		t.Errorf("expected channel_url updated, got %q", updated.ChannelURL)
	}
}

func TestChannelService_Update_ToggleActive(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	testCatID := uuid.New()
	testCatName := fmt.Sprintf("test-toggle-cat-%s", testCatID.String()[:8])
	_, err := pool.Exec(ctx,
		"INSERT INTO categories (id, name, slug) VALUES ($1, $2, $3)",
		testCatID, testCatName, testCatName)
	if err != nil {
		t.Fatalf("failed to create test category: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM channels WHERE category_id = $1", testCatID)
		_, _ = pool.Exec(ctx, "DELETE FROM categories WHERE id = $1", testCatID)
	}()

	svc := NewChannelService(pool)
	ch, err := svc.Create(ctx, CreateChannelRequest{
		CategoryID:  &testCatID,
		Platform:    "facebook",
		ChannelName: "Toggle Active Test",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if !ch.IsActive {
		t.Fatal("expected new channel to be active")
	}

	inactive := false
	updated, err := svc.Update(ctx, ch.ID, UpdateChannelRequest{
		IsActive: &inactive,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.IsActive {
		t.Error("expected channel to be inactive after update")
	}
}

func TestChannelService_Delete(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	testCatID := uuid.New()
	testCatName := fmt.Sprintf("test-delete-cat-%s", testCatID.String()[:8])
	_, err := pool.Exec(ctx,
		"INSERT INTO categories (id, name, slug) VALUES ($1, $2, $3)",
		testCatID, testCatName, testCatName)
	if err != nil {
		t.Fatalf("failed to create test category: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM channels WHERE category_id = $1", testCatID)
		_, _ = pool.Exec(ctx, "DELETE FROM categories WHERE id = $1", testCatID)
	}()

	svc := NewChannelService(pool)
	ch, err := svc.Create(ctx, CreateChannelRequest{
		CategoryID:  &testCatID,
		Platform:    "youtube",
		ChannelName: "Delete Me",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := svc.Delete(ctx, ch.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = svc.GetByID(ctx, ch.ID)
	if err == nil {
		t.Error("expected error fetching deleted channel")
	}
}

func TestChannelService_Delete_NotFound(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	svc := NewChannelService(pool)
	err := svc.Delete(context.Background(), uuid.New())
	if err == nil {
		t.Error("expected error deleting non-existent channel")
	}
}

func TestChannelService_List(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	svc := NewChannelService(pool)
	channels, _, err := svc.List(context.Background(), nil, "", 0, 0)
	if err != nil {
		t.Fatalf("List all failed: %v", err)
	}
	if channels == nil {
		t.Error("expected non-nil slice from List")
	}
}

func TestChannelService_List_FilterPlatform(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	testCatID := uuid.New()
	testCatName := fmt.Sprintf("test-filter-cat-%s", testCatID.String()[:8])
	_, err := pool.Exec(ctx,
		"INSERT INTO categories (id, name, slug) VALUES ($1, $2, $3)",
		testCatID, testCatName, testCatName)
	if err != nil {
		t.Fatalf("failed to create test category: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM channels WHERE category_id = $1", testCatID)
		_, _ = pool.Exec(ctx, "DELETE FROM categories WHERE id = $1", testCatID)
	}()

	svc := NewChannelService(pool)
	ch, err := svc.Create(ctx, CreateChannelRequest{
		CategoryID:  &testCatID,
		Platform:    "youtube",
		ChannelName: "Filter Test YT",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	ytChannels, _, err := svc.List(ctx, nil, "youtube", 0, 0)
	if err != nil {
		t.Fatalf("List by platform failed: %v", err)
	}
	found := false
	for _, c := range ytChannels {
		if c.ID == ch.ID {
			found = true
		}
		if c.Platform != "youtube" {
			t.Errorf("expected platform 'youtube' in filtered result, got %q", c.Platform)
		}
	}
	if !found {
		t.Error("expected created channel in youtube filter results")
	}
}

func TestChannelService_GetByID_NotFound(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	svc := NewChannelService(pool)
	_, err := svc.GetByID(context.Background(), uuid.New())
	if err == nil {
		t.Error("expected error for non-existent channel ID")
	}
}

func TestChannelService_Create_EmptyOptionalFields(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	testCatID := uuid.New()
	testCatName := fmt.Sprintf("test-optional-cat-%s", testCatID.String()[:8])
	_, err := pool.Exec(ctx,
		"INSERT INTO categories (id, name, slug) VALUES ($1, $2, $3)",
		testCatID, testCatName, testCatName)
	if err != nil {
		t.Fatalf("failed to create test category: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM channels WHERE category_id = $1", testCatID)
		_, _ = pool.Exec(ctx, "DELETE FROM categories WHERE id = $1", testCatID)
	}()

	svc := NewChannelService(pool)
	ch, err := svc.Create(ctx, CreateChannelRequest{
		CategoryID:  &testCatID,
		Platform:    "youtube",
		ChannelName: "No Optional Fields",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if ch.ChannelID != "" {
		t.Errorf("expected empty channel_id for nil input, got %q", ch.ChannelID)
	}
	if ch.ChannelURL != "" {
		t.Errorf("expected empty channel_url for nil input, got %q", ch.ChannelURL)
	}
}
