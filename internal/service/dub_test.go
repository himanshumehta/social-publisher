package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

func TestDubService_CreateJob(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewDubService(pool)
	ctx := context.Background()

	// Create test project and video first
	projectID, videoID := createTestProjectAndVideo(t, pool)

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004") // creator

	req := &model.CreateDubJobRequest{
		ProjectID:      projectID,
		VideoID:        videoID,
		TargetLanguage: "hi",
		SourceLanguage: "en",
	}

	job, err := svc.CreateJob(ctx, userID, req)
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	if job.ID == uuid.Nil {
		t.Error("expected non-nil job ID")
	}
	if job.Status != model.DubJobStatusPending {
		t.Errorf("expected status pending, got %s", job.Status)
	}
	if job.TargetLanguage != "hi" {
		t.Errorf("expected target language hi, got %s", job.TargetLanguage)
	}
	if job.SourceLanguage != "en" {
		t.Errorf("expected source language en, got %s", job.SourceLanguage)
	}
	if !job.PreserveBackgroundAudio {
		t.Error("expected preserve_background_audio to be true by default")
	}
}

func TestDubService_UnsupportedLanguage(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewDubService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	req := &model.CreateDubJobRequest{
		ProjectID:      uuid.New(),
		VideoID:        uuid.New(),
		TargetLanguage: "fr", // French not supported
	}

	_, err := svc.CreateJob(ctx, userID, req)
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
}

func TestDubService_BatchCreate(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewDubService(pool)
	ctx := context.Background()

	projectID, videoID := createTestProjectAndVideo(t, pool)
	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	req := &model.BatchDubRequest{
		ProjectID:       projectID,
		VideoID:         videoID,
		TargetLanguages: []string{"hi", "ta", "te"},
	}

	jobs, err := svc.BatchCreate(ctx, userID, req)
	if err != nil {
		t.Fatalf("BatchCreate() error = %v", err)
	}

	if len(jobs) != 3 {
		t.Errorf("expected 3 jobs, got %d", len(jobs))
	}
}

func TestDubService_GetJob(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewDubService(pool)
	ctx := context.Background()

	projectID, videoID := createTestProjectAndVideo(t, pool)
	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	created, err := svc.CreateJob(ctx, userID, &model.CreateDubJobRequest{
		ProjectID:      projectID,
		VideoID:        videoID,
		TargetLanguage: "bn",
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	got, err := svc.GetJob(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("expected job ID %s, got %s", created.ID, got.ID)
	}
}

func TestDubService_ListByProject(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewDubService(pool)
	ctx := context.Background()

	projectID, videoID := createTestProjectAndVideo(t, pool)
	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	// Create 2 jobs
	for _, lang := range []string{"hi", "ta"} {
		_, err := svc.CreateJob(ctx, userID, &model.CreateDubJobRequest{
			ProjectID:      projectID,
			VideoID:        videoID,
			TargetLanguage: lang,
		})
		if err != nil {
			t.Fatalf("CreateJob() error = %v", err)
		}
	}

	jobs, err := svc.ListByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("ListByProject() error = %v", err)
	}
	if len(jobs) < 2 {
		t.Errorf("expected at least 2 jobs, got %d", len(jobs))
	}
}

func TestDubService_Cancel(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewDubService(pool)
	ctx := context.Background()

	projectID, videoID := createTestProjectAndVideo(t, pool)
	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	job, err := svc.CreateJob(ctx, userID, &model.CreateDubJobRequest{
		ProjectID:      projectID,
		VideoID:        videoID,
		TargetLanguage: "mr",
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	if err := svc.Cancel(ctx, userID, job.ID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	got, err := svc.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if got.Status != model.DubJobStatusCancelled {
		t.Errorf("expected status cancelled, got %s", got.Status)
	}
}

func TestDubService_PickPendingAndComplete(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewDubService(pool)
	ctx := context.Background()

	projectID, videoID := createTestProjectAndVideo(t, pool)
	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	_, err := svc.CreateJob(ctx, userID, &model.CreateDubJobRequest{
		ProjectID:      projectID,
		VideoID:        videoID,
		TargetLanguage: "gu",
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	picked, err := svc.PickPending(ctx)
	if err != nil {
		t.Fatalf("PickPending() error = %v", err)
	}
	if picked.Status != model.DubJobStatusProcessing {
		t.Errorf("expected status processing, got %s", picked.Status)
	}

	err = svc.Complete(ctx, picked.ID, "https://s3.example.com/dubbed.mp4", "https://s3.example.com/dubbed.mp3", 10)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	got, err := svc.GetJob(ctx, picked.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if got.Status != model.DubJobStatusCompleted {
		t.Errorf("expected status completed, got %s", got.Status)
	}
	if got.CreditsUsed != 10 {
		t.Errorf("expected 10 credits used, got %d", got.CreditsUsed)
	}
}

func TestDubService_Fail(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewDubService(pool)
	ctx := context.Background()

	projectID, videoID := createTestProjectAndVideo(t, pool)
	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	job, err := svc.CreateJob(ctx, userID, &model.CreateDubJobRequest{
		ProjectID:      projectID,
		VideoID:        videoID,
		TargetLanguage: "kn",
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	// Pick it first
	_, _ = svc.PickPending(ctx)

	err = svc.Fail(ctx, job.ID, "transcription failed")
	if err != nil {
		t.Fatalf("Fail() error = %v", err)
	}

	got, err := svc.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if got.Status != model.DubJobStatusFailed {
		t.Errorf("expected status failed, got %s", got.Status)
	}
}

func TestDubService_CreateOutput(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewDubService(pool)
	ctx := context.Background()

	projectID, videoID := createTestProjectAndVideo(t, pool)
	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	job, err := svc.CreateJob(ctx, userID, &model.CreateDubJobRequest{
		ProjectID:      projectID,
		VideoID:        videoID,
		TargetLanguage: "pa",
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	audioURL := "https://s3.example.com/audio.mp3"
	videoURL := "https://s3.example.com/video.mp4"
	output, err := svc.CreateOutput(ctx, &model.DubOutput{
		DubJobID: job.ID,
		Language: "pa",
		AudioURL: &audioURL,
		VideoURL: &videoURL,
		Status:   "completed",
	})
	if err != nil {
		t.Fatalf("CreateOutput() error = %v", err)
	}
	if output.ID == uuid.Nil {
		t.Error("expected non-nil output ID")
	}

	outputs, err := svc.ListOutputs(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListOutputs() error = %v", err)
	}
	if len(outputs) != 1 {
		t.Errorf("expected 1 output, got %d", len(outputs))
	}
}

func TestDubService_EstimateCredits(t *testing.T) {
	svc := &DubService{}

	tests := []struct {
		name     string
		seconds  int
		expected int
	}{
		{"30 seconds = 1 minute min", 30, 5},
		{"60 seconds = 1 minute", 60, 5},
		{"90 seconds = 2 minutes", 90, 10},
		{"300 seconds = 5 minutes", 300, 25},
		{"0 seconds = 1 minute min", 0, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.EstimateCredits(tt.seconds)
			if got != tt.expected {
				t.Errorf("EstimateCredits(%d) = %d, want %d", tt.seconds, got, tt.expected)
			}
		})
	}
}

func TestDubService_GetSupportedLanguages(t *testing.T) {
	svc := &DubService{}
	langs := svc.GetSupportedLanguages()
	if len(langs) != 9 {
		t.Errorf("expected 9 supported languages, got %d", len(langs))
	}
}

// Helper to create test project and video for dub tests
func createTestProjectAndVideo(t *testing.T, pool *pgxpool.Pool) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	// Find an existing user to satisfy FK constraints
	userID := findUserByRole(t, pool, "creator")

	slug := "dub-test-" + uuid.New().String()[:8]
	var projectID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO projects (title, slug, description, requirements, budget_amount, budget_currency,
			status, creator_id, created_by, priority, tags)
		VALUES ('Dub Test Project', $1, 'Test', '{}', 0, 'INR', 'active', $2, $2, 0, '{}')
		RETURNING id`, slug, userID,
	).Scan(&projectID)
	if err != nil {
		t.Fatalf("create test project: %v", err)
	}

	videoID := createTestVideo(t, pool, projectID, userID, "youtube_16_9")

	return projectID, videoID
}
