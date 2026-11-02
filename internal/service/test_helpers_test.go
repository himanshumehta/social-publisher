package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ---------------------------------------------------------------------------
// Shared test fixture helpers used across multiple test files in this package.
// ---------------------------------------------------------------------------

// findUserByRole looks up a user by role. Skips if none found.
func findUserByRole(t *testing.T, pool *pgxpool.Pool, role string) uuid.UUID {
	t.Helper()
	var userID uuid.UUID
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE role = $1 LIMIT 1`, role,
	).Scan(&userID)
	if err != nil {
		t.Skipf("skipping: no user with role %q: %v", role, err)
	}
	return userID
}

// createTestProject inserts a project with the given status, returning the project ID.
func createTestProject(t *testing.T, pool *pgxpool.Pool, creatorID uuid.UUID, status string) uuid.UUID {
	t.Helper()
	slug := "test-" + uuid.New().String()[:8]
	var projectID uuid.UUID
	err := pool.QueryRow(context.Background(),
		`INSERT INTO projects (title, slug, description, requirements, budget_amount, budget_currency,
		    status, creator_id, created_by, priority, tags)
		 VALUES ($1, $2, '', '{}', 0, 'INR', $3, $4, $4, 0, '{}')
		 RETURNING id`,
		"Test Project "+slug, slug, status, creatorID,
	).Scan(&projectID)
	if err != nil {
		t.Fatalf("failed to create test project: %v", err)
	}
	return projectID
}

// createTestVideo inserts a video for the given project and returns its ID.
func createTestVideo(t *testing.T, pool *pgxpool.Pool, projectID, uploadedBy uuid.UUID, format string) uuid.UUID {
	t.Helper()
	var videoID uuid.UUID
	err := pool.QueryRow(context.Background(),
		`INSERT INTO videos (project_id, uploaded_by, format, version, storage_key, storage_type,
		    file_name, file_size_bytes, mime_type, status, is_current, is_deleted)
		 VALUES ($1, $2, $3, 1, $4, 'hot', 'test.mp4', 1024, 'video/mp4', 'ready', TRUE, FALSE)
		 RETURNING id`,
		projectID, uploadedBy, format, "test/"+uuid.New().String()+".mp4",
	).Scan(&videoID)
	if err != nil {
		t.Fatalf("failed to create test video: %v", err)
	}
	return videoID
}

// createTestChannel inserts a channel (with a dedicated category) and returns its ID.
func createTestChannel(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	slug := "test-cat-" + uuid.New().String()[:8]
	var categoryID uuid.UUID
	err := pool.QueryRow(context.Background(),
		`INSERT INTO categories (name, slug, description) VALUES ($1, $2, 'test')
		 RETURNING id`,
		"Test Category "+slug, slug,
	).Scan(&categoryID)
	if err != nil {
		t.Fatalf("failed to create test category for channel: %v", err)
	}

	var channelID uuid.UUID
	err = pool.QueryRow(context.Background(),
		`INSERT INTO channels (category_id, platform, channel_name, is_active)
		 VALUES ($1, 'youtube', $2, TRUE)
		 RETURNING id`,
		categoryID, "Test Channel "+uuid.New().String()[:8],
	).Scan(&channelID)
	if err != nil {
		t.Fatalf("failed to create test channel: %v", err)
	}
	return channelID
}

// cleanupProject deletes a project and all dependent rows (via CASCADE or manual cleanup).
func cleanupProject(t *testing.T, pool *pgxpool.Pool, projectID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	// Delete dependent tables that may not have ON DELETE CASCADE.
	_, _ = pool.Exec(ctx, `DELETE FROM publish_records WHERE project_id = $1`, projectID)
	_, _ = pool.Exec(ctx, `DELETE FROM channel_targets WHERE project_id = $1`, projectID)
	_, _ = pool.Exec(ctx, `DELETE FROM clip_jobs WHERE project_id = $1`, projectID)
	_, _ = pool.Exec(ctx, `DELETE FROM reviews WHERE project_id = $1`, projectID)
	_, _ = pool.Exec(ctx, `DELETE FROM raw_assets WHERE project_id = $1`, projectID)
	_, _ = pool.Exec(ctx, `DELETE FROM videos WHERE project_id = $1`, projectID)
	_, _ = pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
}

// cleanupChannel deletes a channel. The associated category is left as-is
// (harmless test data).
func cleanupChannel(t *testing.T, pool *pgxpool.Pool, channelID uuid.UUID) {
	t.Helper()
	_, _ = pool.Exec(context.Background(),
		`DELETE FROM channels WHERE id = $1`, channelID)
}
