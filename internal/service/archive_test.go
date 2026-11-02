package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/himanshumehta/social-publisher/internal/model"
)

func TestArchiveInitiate_RequiresPublishedStatus(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	projectID := uuid.New()

	// Cleanup on exit (FK order: archive_tasks -> videos -> projects -> users)
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM archive_tasks WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM videos WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	}()

	// Setup: create user and project with status "active"
	slug := "test-" + uuid.New().String()[:8]
	_, err := pool.Exec(ctx,
		"INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)",
		userID, "archive-test-user", "archive-test-status@test.com", "creator",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO projects (id, title, slug, description, requirements, budget_amount, budget_currency, status, creator_id, created_by, priority, tags, updated_at)
		 VALUES ($1, $2, $3, '', '{}', 0, 'INR', $4, $5, $5, 0, '{}', $6)`,
		projectID, "Test Active Project", slug, "active", userID, time.Now().Add(-15*24*time.Hour),
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	svc := NewArchiveService(pool, nil, nil)
	_, err = svc.InitiateArchive(ctx, userID, projectID)
	if err == nil {
		t.Fatal("expected error for non-published project, got nil")
	}
	if !strings.Contains(err.Error(), "must be published to archive") {
		t.Errorf("expected error about 'must be published to archive', got: %v", err)
	}
}

func TestArchiveInitiate_RequiresMinAge(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	projectID := uuid.New()

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM archive_tasks WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM videos WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	}()

	slug := "test-" + uuid.New().String()[:8]
	_, err := pool.Exec(ctx,
		"INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)",
		userID, "archive-test-user", "archive-test-age@test.com", "creator",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	// Project is published but updated_at is NOW() (too recent)
	_, err = pool.Exec(ctx,
		`INSERT INTO projects (id, title, slug, description, requirements, budget_amount, budget_currency, status, creator_id, created_by, priority, tags, updated_at)
		 VALUES ($1, $2, $3, '', '{}', 0, 'INR', $4, $5, $5, 0, '{}', $6)`,
		projectID, "Test Recent Project", slug, "published", userID, time.Now(),
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	svc := NewArchiveService(pool, nil, nil)
	_, err = svc.InitiateArchive(ctx, userID, projectID)
	if err == nil {
		t.Fatal("expected error for recently updated project, got nil")
	}
	if !strings.Contains(err.Error(), "at least 10 days") {
		t.Errorf("expected error about 'at least 10 days', got: %v", err)
	}
}

func TestArchiveInitiate_NoDuplicateTask(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	projectID := uuid.New()
	videoID := uuid.New()

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM archive_tasks WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM videos WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	}()

	slug := "test-" + uuid.New().String()[:8]
	_, err := pool.Exec(ctx,
		"INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)",
		userID, "archive-test-user", "archive-test-dup@test.com", "creator",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	// Published project, old enough to archive
	_, err = pool.Exec(ctx,
		`INSERT INTO projects (id, title, slug, description, requirements, budget_amount, budget_currency, status, creator_id, created_by, priority, tags, updated_at)
		 VALUES ($1, $2, $3, '', '{}', 0, 'INR', $4, $5, $5, 0, '{}', $6)`,
		projectID, "Test Old Published Project", slug, "published", userID, time.Now().Add(-15*24*time.Hour),
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	// Insert a raw video so InitiateArchive has something to count
	_, err = pool.Exec(ctx,
		`INSERT INTO videos (id, project_id, uploaded_by, format, version, storage_key, storage_type, file_name, file_size_bytes, mime_type, status, is_current, is_deleted)
		 VALUES ($1, $2, $3, 'raw', 1, 'test/key.mp4', 'spaces', 'test.mp4', 1000, 'video/mp4', 'ready', TRUE, FALSE)`,
		videoID, projectID, userID,
	)
	if err != nil {
		t.Fatalf("insert video: %v", err)
	}

	svc := NewArchiveService(pool, nil, nil)

	// First initiation should succeed
	task, err := svc.InitiateArchive(ctx, userID, projectID)
	if err != nil {
		t.Fatalf("first InitiateArchive failed: %v", err)
	}
	if task == nil {
		t.Fatal("expected archive task, got nil")
	}
	if task.Status != model.ArchiveTaskPending {
		t.Errorf("expected status 'pending', got %q", task.Status)
	}

	// Second initiation should fail with duplicate error
	_, err = svc.InitiateArchive(ctx, userID, projectID)
	if err == nil {
		t.Fatal("expected error for duplicate archive task, got nil")
	}
	if !strings.Contains(err.Error(), "already pending") {
		t.Errorf("expected error about 'already pending', got: %v", err)
	}
}

func TestArchiveGetStatus(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	projectID := uuid.New()
	videoID := uuid.New()

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM archive_tasks WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM videos WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	}()

	slug := "test-" + uuid.New().String()[:8]
	_, err := pool.Exec(ctx,
		"INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)",
		userID, "archive-test-user", "archive-test-getstatus@test.com", "creator",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO projects (id, title, slug, description, requirements, budget_amount, budget_currency, status, creator_id, created_by, priority, tags, updated_at)
		 VALUES ($1, $2, $3, '', '{}', 0, 'INR', $4, $5, $5, 0, '{}', $6)`,
		projectID, "Test GetStatus Project", slug, "published", userID, time.Now().Add(-15*24*time.Hour),
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO videos (id, project_id, uploaded_by, format, version, storage_key, storage_type, file_name, file_size_bytes, mime_type, status, is_current, is_deleted)
		 VALUES ($1, $2, $3, 'raw', 1, 'test/status-key.mp4', 'spaces', 'status-test.mp4', 2048, 'video/mp4', 'ready', TRUE, FALSE)`,
		videoID, projectID, userID,
	)
	if err != nil {
		t.Fatalf("insert video: %v", err)
	}

	svc := NewArchiveService(pool, nil, nil)

	// Before any archive, GetArchiveStatus should return nil
	task, err := svc.GetArchiveStatus(ctx, projectID)
	if err != nil {
		t.Fatalf("GetArchiveStatus before initiate: %v", err)
	}
	if task != nil {
		t.Fatalf("expected nil task before initiation, got %+v", task)
	}

	// Initiate an archive
	created, err := svc.InitiateArchive(ctx, userID, projectID)
	if err != nil {
		t.Fatalf("InitiateArchive failed: %v", err)
	}

	// GetArchiveStatus should now return the task
	task, err = svc.GetArchiveStatus(ctx, projectID)
	if err != nil {
		t.Fatalf("GetArchiveStatus after initiate: %v", err)
	}
	if task == nil {
		t.Fatal("expected archive task from GetArchiveStatus, got nil")
	}

	// Verify task fields
	if task.ID != created.ID {
		t.Errorf("expected task ID %s, got %s", created.ID, task.ID)
	}
	if task.ProjectID != projectID {
		t.Errorf("expected project ID %s, got %s", projectID, task.ProjectID)
	}
	if task.InitiatedBy != userID {
		t.Errorf("expected initiated_by %s, got %s", userID, task.InitiatedBy)
	}
	if task.Status != model.ArchiveTaskPending {
		t.Errorf("expected status 'pending', got %q", task.Status)
	}
	if task.TotalFiles != 1 {
		t.Errorf("expected total_files 1, got %d", task.TotalFiles)
	}
	if task.TotalBytes != 2048 {
		t.Errorf("expected total_bytes 2048, got %d", task.TotalBytes)
	}
	if task.FilesProcessed != 0 {
		t.Errorf("expected files_processed 0, got %d", task.FilesProcessed)
	}
	if task.BytesProcessed != 0 {
		t.Errorf("expected bytes_processed 0, got %d", task.BytesProcessed)
	}
	if task.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
	if task.UpdatedAt.IsZero() {
		t.Error("expected non-zero updated_at")
	}
}
