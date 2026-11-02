package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/himanshumehta/social-publisher/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func createTestProjectForClips(t *testing.T, pool *pgxpool.Pool, creatorID uuid.UUID) uuid.UUID {
	t.Helper()
	var projectID uuid.UUID
	err := pool.QueryRow(context.Background(),
		`INSERT INTO projects (title, slug, description, subject_name, subject_type, status, creator_id, created_by, budget_amount, budget_currency)
		 VALUES ($1, $2, 'test', 'test', 'test', 'active', $3, $3, 0, 'INR')
		 RETURNING id`,
		"Clip Test "+uuid.New().String()[:8],
		"clip-test-"+uuid.New().String()[:8],
		creatorID,
	).Scan(&projectID)
	if err != nil {
		t.Fatalf("failed to create test project: %v", err)
	}
	return projectID
}

func createTestVideoForClips(t *testing.T, pool *pgxpool.Pool, projectID, uploadedBy uuid.UUID) uuid.UUID {
	t.Helper()
	var videoID uuid.UUID
	err := pool.QueryRow(context.Background(),
		`INSERT INTO videos (project_id, uploaded_by, format, version, storage_key, storage_type, file_name, file_size_bytes, mime_type, status, is_current)
		 VALUES ($1, $2, 'raw', 1, $3, 'hot', 'raw.mp4', 1024, 'video/mp4', 'ready', TRUE)
		 RETURNING id`,
		projectID, uploadedBy, "test/"+uuid.New().String()+".mp4",
	).Scan(&videoID)
	if err != nil {
		t.Fatalf("failed to create test video: %v", err)
	}
	return videoID
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestClipJobService_Create(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	var userID uuid.UUID
	err := pool.QueryRow(ctx, "SELECT id FROM users WHERE role = 'creator' LIMIT 1").Scan(&userID)
	if err != nil {
		t.Skipf("skipping: no creator user in test database: %v", err)
	}

	projectID := createTestProjectForClips(t, pool, userID)
	videoID := createTestVideoForClips(t, pool, projectID, userID)

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM videos WHERE clip_job_id IN (SELECT id FROM clip_jobs WHERE project_id = $1)", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM clip_jobs WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM videos WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)
	}()

	svc := NewClipJobService(pool)

	job, err := svc.Create(ctx, projectID, userID, CreateClipJobRequest{
		SourceVideoID: videoID,
		Provider:      "test-provider",
		Config:        []byte(`{"clip_count":5}`),
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if job.ID == uuid.Nil {
		t.Fatal("expected job to have an ID")
	}
	if job.ProjectID != projectID {
		t.Errorf("expected project_id %s, got %s", projectID, job.ProjectID)
	}
	if job.SourceVideoID != videoID {
		t.Errorf("expected source_video_id %s, got %s", videoID, job.SourceVideoID)
	}
	if job.Status != model.ClipJobPending {
		t.Errorf("expected status %q, got %q", model.ClipJobPending, job.Status)
	}
	if job.Provider != "test-provider" {
		t.Errorf("expected provider %q, got %q", "test-provider", job.Provider)
	}
}

func TestClipJobService_ListByProject(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	var userID uuid.UUID
	err := pool.QueryRow(ctx, "SELECT id FROM users WHERE role = 'creator' LIMIT 1").Scan(&userID)
	if err != nil {
		t.Skipf("skipping: no creator user in test database: %v", err)
	}

	projectID := createTestProjectForClips(t, pool, userID)
	videoID := createTestVideoForClips(t, pool, projectID, userID)

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM videos WHERE clip_job_id IN (SELECT id FROM clip_jobs WHERE project_id = $1)", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM clip_jobs WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM videos WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)
	}()

	svc := NewClipJobService(pool)

	// Create three jobs with a small delay to ensure created_at ordering.
	for i := 0; i < 3; i++ {
		_, err := svc.Create(ctx, projectID, userID, CreateClipJobRequest{
			SourceVideoID: videoID,
			Provider:      "test-provider",
		})
		if err != nil {
			t.Fatalf("Create #%d failed: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	jobs, err := svc.ListByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("ListByProject failed: %v", err)
	}

	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}

	// Verify DESC ordering by created_at.
	for i := 1; i < len(jobs); i++ {
		if jobs[i].CreatedAt.After(jobs[i-1].CreatedAt) {
			t.Errorf("expected created_at DESC ordering; job[%d]=%v is after job[%d]=%v",
				i, jobs[i].CreatedAt, i-1, jobs[i-1].CreatedAt)
		}
	}
}

func TestClipJobService_PickPending(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	var userID uuid.UUID
	err := pool.QueryRow(ctx, "SELECT id FROM users WHERE role = 'creator' LIMIT 1").Scan(&userID)
	if err != nil {
		t.Skipf("skipping: no creator user in test database: %v", err)
	}

	projectID := createTestProjectForClips(t, pool, userID)
	videoID := createTestVideoForClips(t, pool, projectID, userID)

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM videos WHERE clip_job_id IN (SELECT id FROM clip_jobs WHERE project_id = $1)", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM clip_jobs WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM videos WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)
	}()

	svc := NewClipJobService(pool)

	created, err := svc.Create(ctx, projectID, userID, CreateClipJobRequest{
		SourceVideoID: videoID,
		Provider:      "test-provider",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	t.Run("picks pending job and sets status to processing", func(t *testing.T) {
		picked, err := svc.PickPending(ctx)
		if err != nil {
			t.Fatalf("PickPending failed: %v", err)
		}
		if picked == nil {
			t.Fatal("expected a job, got nil")
		}
		if picked.ID != created.ID {
			t.Errorf("expected job ID %s, got %s", created.ID, picked.ID)
		}
		if picked.Status != model.ClipJobProcessing {
			t.Errorf("expected status %q, got %q", model.ClipJobProcessing, picked.Status)
		}
		if picked.StartedAt == nil {
			t.Error("expected started_at to be set")
		}
	})

	t.Run("returns nil when no pending jobs", func(t *testing.T) {
		picked, err := svc.PickPending(ctx)
		if err != nil {
			t.Fatalf("PickPending failed: %v", err)
		}
		if picked != nil {
			t.Errorf("expected nil when no pending jobs, got job ID %s", picked.ID)
		}
	})
}

func TestClipJobService_Complete(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	var userID uuid.UUID
	err := pool.QueryRow(ctx, "SELECT id FROM users WHERE role = 'creator' LIMIT 1").Scan(&userID)
	if err != nil {
		t.Skipf("skipping: no creator user in test database: %v", err)
	}

	projectID := createTestProjectForClips(t, pool, userID)
	videoID := createTestVideoForClips(t, pool, projectID, userID)

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM videos WHERE clip_job_id IN (SELECT id FROM clip_jobs WHERE project_id = $1)", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM clip_jobs WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM videos WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)
	}()

	svc := NewClipJobService(pool)

	created, err := svc.Create(ctx, projectID, userID, CreateClipJobRequest{
		SourceVideoID: videoID,
		Provider:      "test-provider",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Move to processing first.
	_, err = svc.PickPending(ctx)
	if err != nil {
		t.Fatalf("PickPending failed: %v", err)
	}

	err = svc.Complete(ctx, created.ID, 3)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	// Verify directly from DB.
	var status string
	var clipCount int
	var completedAt *time.Time
	err = pool.QueryRow(ctx,
		`SELECT status, clip_count, completed_at FROM clip_jobs WHERE id = $1`,
		created.ID,
	).Scan(&status, &clipCount, &completedAt)
	if err != nil {
		t.Fatalf("failed to query completed job: %v", err)
	}
	if status != string(model.ClipJobCompleted) {
		t.Errorf("expected status %q, got %q", model.ClipJobCompleted, status)
	}
	if clipCount != 3 {
		t.Errorf("expected clip_count 3, got %d", clipCount)
	}
	if completedAt == nil {
		t.Error("expected completed_at to be set")
	}
}

func TestClipJobService_Fail(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	var userID uuid.UUID
	err := pool.QueryRow(ctx, "SELECT id FROM users WHERE role = 'creator' LIMIT 1").Scan(&userID)
	if err != nil {
		t.Skipf("skipping: no creator user in test database: %v", err)
	}

	projectID := createTestProjectForClips(t, pool, userID)
	videoID := createTestVideoForClips(t, pool, projectID, userID)

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM videos WHERE clip_job_id IN (SELECT id FROM clip_jobs WHERE project_id = $1)", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM clip_jobs WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM videos WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)
	}()

	svc := NewClipJobService(pool)

	created, err := svc.Create(ctx, projectID, userID, CreateClipJobRequest{
		SourceVideoID: videoID,
		Provider:      "test-provider",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Move to processing first.
	_, err = svc.PickPending(ctx)
	if err != nil {
		t.Fatalf("PickPending failed: %v", err)
	}

	failMsg := "provider timeout: context deadline exceeded"
	err = svc.Fail(ctx, created.ID, failMsg)
	if err != nil {
		t.Fatalf("Fail failed: %v", err)
	}

	// Verify directly from DB.
	var status string
	var errorMessage *string
	var completedAt *time.Time
	err = pool.QueryRow(ctx,
		`SELECT status, error_message, completed_at FROM clip_jobs WHERE id = $1`,
		created.ID,
	).Scan(&status, &errorMessage, &completedAt)
	if err != nil {
		t.Fatalf("failed to query failed job: %v", err)
	}
	if status != string(model.ClipJobFailed) {
		t.Errorf("expected status %q, got %q", model.ClipJobFailed, status)
	}
	if errorMessage == nil || *errorMessage != failMsg {
		t.Errorf("expected error_message %q, got %v", failMsg, errorMessage)
	}
	if completedAt == nil {
		t.Error("expected completed_at to be set")
	}
}

func TestClipJobService_ListClipsByJob(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	var userID uuid.UUID
	err := pool.QueryRow(ctx, "SELECT id FROM users WHERE role = 'creator' LIMIT 1").Scan(&userID)
	if err != nil {
		t.Skipf("skipping: no creator user in test database: %v", err)
	}

	projectID := createTestProjectForClips(t, pool, userID)
	videoID := createTestVideoForClips(t, pool, projectID, userID)

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM videos WHERE clip_job_id IN (SELECT id FROM clip_jobs WHERE project_id = $1)", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM clip_jobs WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM videos WHERE project_id = $1", projectID)
		_, _ = pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)
	}()

	svc := NewClipJobService(pool)

	job, err := svc.Create(ctx, projectID, userID, CreateClipJobRequest{
		SourceVideoID: videoID,
		Provider:      "test-provider",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Insert clip videos with different virality scores.
	scores := []int{42, 88, 15, 71}
	for i, score := range scores {
		_, err := pool.Exec(ctx,
			`INSERT INTO videos (project_id, uploaded_by, format, version, storage_key, storage_type,
			   file_name, file_size_bytes, mime_type, status, is_current,
			   source_video_id, clip_job_id, virality_score)
			 VALUES ($1, $2, 'youtube_short', $3, $4, 'hot', $5, 512, 'video/mp4', 'ready', FALSE,
			   $6, $7, $8)`,
			projectID, userID, i+1,
			"test/clip-"+uuid.New().String()+".mp4",
			"clip.mp4",
			videoID, job.ID, score,
		)
		if err != nil {
			t.Fatalf("failed to insert clip video with score %d: %v", score, err)
		}
	}

	clips, err := svc.ListClipsByJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListClipsByJob failed: %v", err)
	}

	if len(clips) != 4 {
		t.Fatalf("expected 4 clips, got %d", len(clips))
	}

	// Verify DESC ordering by virality_score.
	for i := 1; i < len(clips); i++ {
		prev := clips[i-1].ViralityScore
		curr := clips[i].ViralityScore
		if prev == nil || curr == nil {
			t.Fatalf("expected virality_score to be set on all clips")
		}
		if *curr > *prev {
			t.Errorf("expected virality_score DESC ordering; clip[%d]=%d > clip[%d]=%d",
				i, *curr, i-1, *prev)
		}
	}

	// Verify the top clip has score 88.
	if *clips[0].ViralityScore != 88 {
		t.Errorf("expected top clip virality_score 88, got %d", *clips[0].ViralityScore)
	}
}

// Note: contains() helper is defined in upload_test.go (same package).
