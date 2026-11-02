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
