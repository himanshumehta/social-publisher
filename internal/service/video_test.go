package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// setupTestPool is defined in project_test.go (same package).

func TestVideoSoftDelete_AsUploader(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	userID := uuid.New()
	projectID := uuid.New()
	videoID := uuid.New()

	// Insert a test user (creator role).
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, name, role)
		 VALUES ($1, $2, $3, 'creator')`,
		userID, userID.String()+"@test.com", "Test Creator")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)

	// Insert a project with status = 'active'.
	_, err = pool.Exec(ctx,
		`INSERT INTO projects (id, title, slug, description, requirements, budget_amount, budget_currency,
		    status, creator_id, created_by, priority, tags)
		 VALUES ($1, $2, $3, '', '{}', 0, 'INR', 'active', $4, $4, 0, '{}')`,
		projectID, "Test Project", "test-project-"+projectID.String(), userID)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)

	// Insert a video WITHOUT thumbnail_key (so s3 delete is skipped).
	_, err = pool.Exec(ctx,
		`INSERT INTO videos (id, project_id, uploaded_by, format, version,
		    storage_key, storage_type, file_name, file_size_bytes, mime_type, status, is_current, is_deleted)
		 VALUES ($1, $2, $3, 'raw', 1, 'test/key.mp4', 'hot', 'test.mp4', 1024, 'video/mp4', 'ready', TRUE, FALSE)`,
		videoID, projectID, userID)
	if err != nil {
		t.Fatalf("insert video: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM videos WHERE id = $1", videoID)

	// Create service with nil s3 (safe because thumbnail_key is NULL).
	svc := NewVideoService(pool, nil)

	err = svc.SoftDelete(ctx, videoID, userID, "creator")
	if err != nil {
		t.Fatalf("SoftDelete as uploader should succeed, got: %v", err)
	}

	// Verify is_deleted = true via direct SQL.
	var isDeleted bool
	err = pool.QueryRow(ctx,
		"SELECT is_deleted FROM videos WHERE id = $1", videoID).Scan(&isDeleted)
	if err != nil {
		t.Fatalf("query is_deleted: %v", err)
	}
	if !isDeleted {
		t.Error("expected is_deleted to be true after SoftDelete")
	}
}

func TestVideoSoftDelete_WrongUser(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	uploaderID := uuid.New()
	otherUserID := uuid.New()
	projectID := uuid.New()
	videoID := uuid.New()

	// Insert the uploader user.
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, name, role)
		 VALUES ($1, $2, $3, 'creator')`,
		uploaderID, uploaderID.String()+"@test.com", "Uploader")
	if err != nil {
		t.Fatalf("insert uploader: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM users WHERE id = $1", uploaderID)

	// Insert the other user who will try to delete.
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, email, name, role)
		 VALUES ($1, $2, $3, 'creator')`,
		otherUserID, otherUserID.String()+"@test.com", "Other User")
	if err != nil {
		t.Fatalf("insert other user: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM users WHERE id = $1", otherUserID)

	// Insert a project with status = 'active'.
	_, err = pool.Exec(ctx,
		`INSERT INTO projects (id, title, slug, description, requirements, budget_amount, budget_currency,
		    status, creator_id, created_by, priority, tags)
		 VALUES ($1, $2, $3, '', '{}', 0, 'INR', 'active', $4, $4, 0, '{}')`,
		projectID, "Test Project", "test-project-"+projectID.String(), uploaderID)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)

	// Insert video owned by the uploader.
	_, err = pool.Exec(ctx,
		`INSERT INTO videos (id, project_id, uploaded_by, format, version,
		    storage_key, storage_type, file_name, file_size_bytes, mime_type, status, is_current, is_deleted)
		 VALUES ($1, $2, $3, 'raw', 1, 'test/key.mp4', 'hot', 'test.mp4', 1024, 'video/mp4', 'ready', TRUE, FALSE)`,
		videoID, projectID, uploaderID)
	if err != nil {
		t.Fatalf("insert video: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM videos WHERE id = $1", videoID)

	svc := NewVideoService(pool, nil)

	// Try to delete as a different non-admin user.
	err = svc.SoftDelete(ctx, videoID, otherUserID, "creator")
	if err == nil {
		t.Fatal("expected error when non-uploader tries to delete, got nil")
	}

	expected := "forbidden: only the uploader or admin can delete this video"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}

	// Verify video is NOT deleted.
	var isDeleted bool
	err = pool.QueryRow(ctx,
		"SELECT is_deleted FROM videos WHERE id = $1", videoID).Scan(&isDeleted)
	if err != nil {
		t.Fatalf("query is_deleted: %v", err)
	}
	if isDeleted {
		t.Error("expected is_deleted to be false (wrong user should not delete)")
	}
}

func TestVideoSoftDelete_NonActiveProject(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	userID := uuid.New()
	projectID := uuid.New()
	videoID := uuid.New()

	// Insert user.
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, name, role)
		 VALUES ($1, $2, $3, 'creator')`,
		userID, userID.String()+"@test.com", "Test Creator")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)

	// Insert project with status = 'published' (not active).
	_, err = pool.Exec(ctx,
		`INSERT INTO projects (id, title, slug, description, requirements, budget_amount, budget_currency,
		    status, creator_id, created_by, priority, tags)
		 VALUES ($1, $2, $3, '', '{}', 0, 'INR', 'published', $4, $4, 0, '{}')`,
		projectID, "Published Project", "test-project-"+projectID.String(), userID)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)

	// Insert video.
	_, err = pool.Exec(ctx,
		`INSERT INTO videos (id, project_id, uploaded_by, format, version,
		    storage_key, storage_type, file_name, file_size_bytes, mime_type, status, is_current, is_deleted)
		 VALUES ($1, $2, $3, 'raw', 1, 'test/key.mp4', 'hot', 'test.mp4', 1024, 'video/mp4', 'ready', TRUE, FALSE)`,
		videoID, projectID, userID)
	if err != nil {
		t.Fatalf("insert video: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM videos WHERE id = $1", videoID)

	svc := NewVideoService(pool, nil)

	// Non-admin cannot delete when project is not active.
	err = svc.SoftDelete(ctx, videoID, userID, "creator")
	if err == nil {
		t.Fatal("expected error when project is not active, got nil")
	}

	expected := "cannot delete files once project is submitted for review"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestVideoSoftDelete_AdminOverridesProjectStatus(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	creatorID := uuid.New()
	adminID := uuid.New()
	projectID := uuid.New()
	videoID := uuid.New()

	// Insert creator user.
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, name, role)
		 VALUES ($1, $2, $3, 'creator')`,
		creatorID, creatorID.String()+"@test.com", "Creator")
	if err != nil {
		t.Fatalf("insert creator: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM users WHERE id = $1", creatorID)

	// Insert admin user.
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, email, name, role)
		 VALUES ($1, $2, $3, 'admin')`,
		adminID, adminID.String()+"@test.com", "Admin User")
	if err != nil {
		t.Fatalf("insert admin: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM users WHERE id = $1", adminID)

	// Insert project with status = 'published' (non-active).
	_, err = pool.Exec(ctx,
		`INSERT INTO projects (id, title, slug, description, requirements, budget_amount, budget_currency,
		    status, creator_id, created_by, priority, tags)
		 VALUES ($1, $2, $3, '', '{}', 0, 'INR', 'published', $4, $4, 0, '{}')`,
		projectID, "Published Project", "test-project-"+projectID.String(), creatorID)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)

	// Insert video uploaded by creator.
	_, err = pool.Exec(ctx,
		`INSERT INTO videos (id, project_id, uploaded_by, format, version,
		    storage_key, storage_type, file_name, file_size_bytes, mime_type, status, is_current, is_deleted)
		 VALUES ($1, $2, $3, 'raw', 1, 'test/key.mp4', 'hot', 'test.mp4', 1024, 'video/mp4', 'ready', TRUE, FALSE)`,
		videoID, projectID, creatorID)
	if err != nil {
		t.Fatalf("insert video: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM videos WHERE id = $1", videoID)

	svc := NewVideoService(pool, nil)

	// Admin can delete even when project is not active and they are not the uploader.
	err = svc.SoftDelete(ctx, videoID, adminID, "admin")
	if err != nil {
		t.Fatalf("admin SoftDelete should succeed regardless of project status, got: %v", err)
	}

	// Verify is_deleted = true.
	var isDeleted bool
	err = pool.QueryRow(ctx,
		"SELECT is_deleted FROM videos WHERE id = $1", videoID).Scan(&isDeleted)
	if err != nil {
		t.Fatalf("query is_deleted: %v", err)
	}
	if !isDeleted {
		t.Error("expected is_deleted to be true after admin SoftDelete")
	}
}

func TestVideoSoftDelete_AlreadyDeleted(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	userID := uuid.New()
	projectID := uuid.New()
	videoID := uuid.New()

	// Insert user.
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, name, role)
		 VALUES ($1, $2, $3, 'creator')`,
		userID, userID.String()+"@test.com", "Test Creator")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)

	// Insert project.
	_, err = pool.Exec(ctx,
		`INSERT INTO projects (id, title, slug, description, requirements, budget_amount, budget_currency,
		    status, creator_id, created_by, priority, tags)
		 VALUES ($1, $2, $3, '', '{}', 0, 'INR', 'active', $4, $4, 0, '{}')`,
		projectID, "Test Project", "test-project-"+projectID.String(), userID)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)

	// Insert video that is already soft-deleted.
	_, err = pool.Exec(ctx,
		`INSERT INTO videos (id, project_id, uploaded_by, format, version,
		    storage_key, storage_type, file_name, file_size_bytes, mime_type, status, is_current, is_deleted)
		 VALUES ($1, $2, $3, 'raw', 1, 'test/key.mp4', 'hot', 'test.mp4', 1024, 'video/mp4', 'ready', TRUE, TRUE)`,
		videoID, projectID, userID)
	if err != nil {
		t.Fatalf("insert video: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM videos WHERE id = $1", videoID)

	svc := NewVideoService(pool, nil)

	// Attempt to delete an already-deleted video should return "not found".
	err = svc.SoftDelete(ctx, videoID, userID, "creator")
	if err == nil {
		t.Fatal("expected error when deleting already-deleted video, got nil")
	}

	expected := "video not found"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestVideoListByProject_CountExcludesDeleted(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	userID := uuid.New()
	projectID := uuid.New()
	video1 := uuid.New()
	video2 := uuid.New()
	video3 := uuid.New()

	// Insert user.
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, name, role)
		 VALUES ($1, $2, $3, 'creator')`,
		userID, userID.String()+"@test.com", "Test Creator")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)

	// Insert project.
	_, err = pool.Exec(ctx,
		`INSERT INTO projects (id, title, slug, description, requirements, budget_amount, budget_currency,
		    status, creator_id, created_by, priority, tags)
		 VALUES ($1, $2, $3, '', '{}', 0, 'INR', 'active', $4, $4, 0, '{}')`,
		projectID, "Count Test Project", "test-project-"+projectID.String(), userID)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)

	// Insert 3 videos: 2 active, 1 soft-deleted.
	for i, vid := range []uuid.UUID{video1, video2, video3} {
		isDeleted := i == 2 // third video is deleted
		_, err = pool.Exec(ctx,
			`INSERT INTO videos (id, project_id, uploaded_by, format, version,
			    storage_key, storage_type, file_name, file_size_bytes, mime_type, status, is_current, is_deleted)
			 VALUES ($1, $2, $3, 'raw', $4, $5, 'hot', 'test.mp4', 1024, 'video/mp4', 'ready', TRUE, $6)`,
			vid, projectID, userID, i+1, "test/key-"+vid.String()+".mp4", isDeleted)
		if err != nil {
			t.Fatalf("insert video %d: %v", i+1, err)
		}
		defer pool.Exec(ctx, "DELETE FROM videos WHERE id = $1", vid)
	}

	// Verify count via direct SQL (same query the service uses), because
	// ListByProject calls s3.PresignGetObject which requires a real S3 client.
	var total int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM videos WHERE project_id = $1 AND is_deleted = FALSE`,
		projectID).Scan(&total)
	if err != nil {
		t.Fatalf("count query: %v", err)
	}

	if total != 2 {
		t.Errorf("expected count of 2 (excluding soft-deleted), got %d", total)
	}
}

func TestVideoSoftDelete_SuperAdminCanDelete(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	creatorID := uuid.New()
	superAdminID := uuid.New()
	projectID := uuid.New()
	videoID := uuid.New()

	// Insert creator.
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, name, role)
		 VALUES ($1, $2, $3, 'creator')`,
		creatorID, creatorID.String()+"@test.com", "Creator")
	if err != nil {
		t.Fatalf("insert creator: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM users WHERE id = $1", creatorID)

	// Insert super_admin.
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, email, name, role)
		 VALUES ($1, $2, $3, 'super_admin')`,
		superAdminID, superAdminID.String()+"@test.com", "Super Admin")
	if err != nil {
		t.Fatalf("insert super_admin: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM users WHERE id = $1", superAdminID)

	// Insert project with status = 'in_review'.
	_, err = pool.Exec(ctx,
		`INSERT INTO projects (id, title, slug, description, requirements, budget_amount, budget_currency,
		    status, creator_id, created_by, priority, tags)
		 VALUES ($1, $2, $3, '', '{}', 0, 'INR', 'in_review', $4, $4, 0, '{}')`,
		projectID, "InReview Project", "test-project-"+projectID.String(), creatorID)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)

	// Insert video.
	_, err = pool.Exec(ctx,
		`INSERT INTO videos (id, project_id, uploaded_by, format, version,
		    storage_key, storage_type, file_name, file_size_bytes, mime_type, status, is_current, is_deleted)
		 VALUES ($1, $2, $3, 'raw', 1, 'test/key.mp4', 'hot', 'test.mp4', 1024, 'video/mp4', 'ready', TRUE, FALSE)`,
		videoID, projectID, creatorID)
	if err != nil {
		t.Fatalf("insert video: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM videos WHERE id = $1", videoID)

	svc := NewVideoService(pool, nil)

	// super_admin can delete regardless of project status or uploader.
	err = svc.SoftDelete(ctx, videoID, superAdminID, "super_admin")
	if err != nil {
		t.Fatalf("super_admin SoftDelete should succeed, got: %v", err)
	}

	var isDeleted bool
	err = pool.QueryRow(ctx,
		"SELECT is_deleted FROM videos WHERE id = $1", videoID).Scan(&isDeleted)
	if err != nil {
		t.Fatalf("query is_deleted: %v", err)
	}
	if !isDeleted {
		t.Error("expected is_deleted to be true after super_admin SoftDelete")
	}
}
