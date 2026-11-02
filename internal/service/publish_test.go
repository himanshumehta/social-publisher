package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// skipIfMigrationMissing skips the test if the error indicates missing columns from migration 000006.
func skipIfMigrationMissing(t *testing.T, err error) {
	t.Helper()
	if err != nil && (strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "42703")) {
		t.Skipf("skipping: migration 000006 not applied: %v", err)
	}
}

func TestPublishService_CreateRecord(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	var projectID uuid.UUID
	err := pool.QueryRow(ctx,
		"SELECT id FROM projects WHERE status = 'published' LIMIT 1").Scan(&projectID)
	if err != nil {
		t.Skipf("skipping: no published project in test database: %v", err)
	}

	var channelID uuid.UUID
	err = pool.QueryRow(ctx,
		"SELECT id FROM channels WHERE is_active = TRUE LIMIT 1").Scan(&channelID)
	if err != nil {
		t.Skipf("skipping: no active channels in test database: %v", err)
	}

	svc := NewPublishService(pool)
	record, err := svc.CreateRecord(ctx, CreatePublishRecordRequest{
		ProjectID:   projectID,
		ChannelID:   channelID,
		PlatformURL: "https://youtube.com/watch?v=test123",
		Title:       "Test Publish Record",
		Visibility:  "public",
	})
	skipIfMigrationMissing(t, err)
	if err != nil {
		t.Fatalf("CreateRecord failed: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM publish_records WHERE id = $1", record.ID)
		_, _ = pool.Exec(ctx, "DELETE FROM notifications WHERE link = $1",
			"/projects/"+projectID.String())
	}()

	if record.ID == uuid.Nil {
		t.Fatal("expected publish record to have an ID")
	}
	if record.PlatformURL != "https://youtube.com/watch?v=test123" {
		t.Errorf("expected platform_url, got %q", record.PlatformURL)
	}
	if record.Status != "published" {
		t.Errorf("expected status 'published', got %q", record.Status)
	}
	if record.ChannelName == "" {
		t.Error("expected channel_name to be populated")
	}
	if record.Platform == "" {
		t.Error("expected platform to be populated")
	}
	if record.Title != "Test Publish Record" {
		t.Errorf("expected title 'Test Publish Record', got %q", record.Title)
	}
}

func TestPublishService_CreateRecord_RequiresPublishedStatus(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	var projectID uuid.UUID
	err := pool.QueryRow(ctx,
		"SELECT id FROM projects WHERE status != 'published' LIMIT 1").Scan(&projectID)
	if err != nil {
		t.Skipf("skipping: no non-published project in test database: %v", err)
	}

	var channelID uuid.UUID
	err = pool.QueryRow(ctx,
		"SELECT id FROM channels WHERE is_active = TRUE LIMIT 1").Scan(&channelID)
	if err != nil {
		t.Skipf("skipping: no active channels in test database: %v", err)
	}

	svc := NewPublishService(pool)
	_, err = svc.CreateRecord(ctx, CreatePublishRecordRequest{
		ProjectID:   projectID,
		ChannelID:   channelID,
		PlatformURL: "https://youtube.com/watch?v=test",
	})
	if err == nil {
		t.Error("expected error when project is not published")
	}
}

func TestPublishService_CreateRecord_RequiresPlatformURL(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	svc := NewPublishService(pool)
	_, err := svc.CreateRecord(context.Background(), CreatePublishRecordRequest{
		ProjectID: uuid.New(),
		ChannelID: uuid.New(),
	})
	if err == nil {
		t.Error("expected error when platform_url is empty")
	}
}

func TestPublishService_ListByProject(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	var projectID uuid.UUID
	err := pool.QueryRow(ctx,
		"SELECT id FROM projects LIMIT 1").Scan(&projectID)
	if err != nil {
		t.Skipf("skipping: no projects in test database: %v", err)
	}

	svc := NewPublishService(pool)
	records, _, err := svc.ListByProject(ctx, projectID, 0, 0)
	skipIfMigrationMissing(t, err)
	if err != nil {
		t.Fatalf("ListByProject failed: %v", err)
	}
	if records == nil {
		t.Error("expected non-nil slice from ListByProject")
	}
}

func TestPublishService_DeleteRecord(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	var projectID uuid.UUID
	err := pool.QueryRow(ctx,
		"SELECT id FROM projects WHERE status = 'published' LIMIT 1").Scan(&projectID)
	if err != nil {
		t.Skipf("skipping: no published project in test database: %v", err)
	}

	var channelID uuid.UUID
	err = pool.QueryRow(ctx,
		"SELECT id FROM channels WHERE is_active = TRUE LIMIT 1").Scan(&channelID)
	if err != nil {
		t.Skipf("skipping: no active channels in test database: %v", err)
	}

	svc := NewPublishService(pool)
	record, err := svc.CreateRecord(ctx, CreatePublishRecordRequest{
		ProjectID:   projectID,
		ChannelID:   channelID,
		PlatformURL: "https://youtube.com/watch?v=deleteme",
	})
	skipIfMigrationMissing(t, err)
	if err != nil {
		t.Fatalf("CreateRecord failed: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM notifications WHERE link = $1",
			"/projects/"+projectID.String())
	}()

	if err := svc.DeleteRecord(ctx, record.ID); err != nil {
		t.Fatalf("DeleteRecord failed: %v", err)
	}

	records, _, err := svc.ListByProject(ctx, projectID, 0, 0)
	if err != nil {
		t.Fatalf("ListByProject failed: %v", err)
	}
	for _, r := range records {
		if r.ID == record.ID {
			t.Error("expected deleted record to be absent from list")
		}
	}
}

func TestPublishService_DeleteRecord_NotFound(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	svc := NewPublishService(pool)
	err := svc.DeleteRecord(context.Background(), uuid.New())
	if err == nil {
		t.Error("expected error deleting non-existent publish record")
	}
}

func TestPublishService_GetSuggestedChannels(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	var projectID uuid.UUID
	err := pool.QueryRow(ctx,
		"SELECT id FROM projects LIMIT 1").Scan(&projectID)
	if err != nil {
		t.Skipf("skipping: no projects in test database: %v", err)
	}

	svc := NewPublishService(pool)
	suggestions, err := svc.GetSuggestedChannels(ctx, projectID)
	if err != nil {
		t.Fatalf("GetSuggestedChannels failed: %v", err)
	}
	if suggestions == nil {
		t.Error("expected non-nil slice from GetSuggestedChannels")
	}

	for _, s := range suggestions {
		if s.Channel.ID == uuid.Nil {
			t.Error("expected suggestion channel to have an ID")
		}
		if len(s.MatchingFormats) == 0 {
			t.Error("expected suggestion to have matching formats")
		}
		if s.Reason == "" {
			t.Error("expected suggestion to have a reason")
		}
	}
}

func TestPublishService_GetSuggestedChannels_NotFound(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	svc := NewPublishService(pool)
	_, err := svc.GetSuggestedChannels(context.Background(), uuid.New())
	if err == nil {
		t.Error("expected error for non-existent project")
	}
}

func TestPublishService_UpdateRecord(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	var projectID uuid.UUID
	err := pool.QueryRow(ctx,
		"SELECT id FROM projects WHERE status = 'published' LIMIT 1").Scan(&projectID)
	if err != nil {
		t.Skipf("skipping: no published project in test database: %v", err)
	}

	var channelID uuid.UUID
	err = pool.QueryRow(ctx,
		"SELECT id FROM channels WHERE is_active = TRUE LIMIT 1").Scan(&channelID)
	if err != nil {
		t.Skipf("skipping: no active channels in test database: %v", err)
	}

	svc := NewPublishService(pool)
	record, err := svc.CreateRecord(ctx, CreatePublishRecordRequest{
		ProjectID:   projectID,
		ChannelID:   channelID,
		PlatformURL: "https://youtube.com/watch?v=updateme",
		Title:       "Before",
	})
	skipIfMigrationMissing(t, err)
	if err != nil {
		t.Fatalf("CreateRecord failed: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM publish_records WHERE id = $1", record.ID)
		_, _ = pool.Exec(ctx, "DELETE FROM notifications WHERE link = $1",
			"/projects/"+projectID.String())
	}()

	newTitle := "After Update"
	newURL := "https://youtube.com/watch?v=updated"
	updated, err := svc.UpdateRecord(ctx, record.ID, UpdatePublishRecordRequest{
		Title:       &newTitle,
		PlatformURL: &newURL,
	})
	if err != nil {
		t.Fatalf("UpdateRecord failed: %v", err)
	}
	if updated.Title != "After Update" {
		t.Errorf("expected title 'After Update', got %q", updated.Title)
	}
	if updated.PlatformURL != "https://youtube.com/watch?v=updated" {
		t.Errorf("expected updated platform_url, got %q", updated.PlatformURL)
	}
}

func TestPublishService_UpdateRecord_NoFields(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	svc := NewPublishService(pool)
	_, err := svc.UpdateRecord(context.Background(), uuid.New(), UpdatePublishRecordRequest{})
	if err == nil {
		t.Error("expected error when no fields provided")
	}
}

func TestFormatPlatformMapping(t *testing.T) {
	tests := []struct {
		format   string
		platform string
	}{
		{"youtube_16_9", "youtube"},
		{"youtube_short", "youtube"},
		{"instagram_reel", "instagram"},
		{"facebook_feed", "facebook"},
		{"facebook_reel", "facebook"},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			got, ok := formatPlatform[tt.format]
			if !ok {
				t.Errorf("format %q not found in formatPlatform map", tt.format)
				return
			}
			if got != tt.platform {
				t.Errorf("expected platform %q for format %q, got %q", tt.platform, tt.format, got)
			}
		})
	}
}
